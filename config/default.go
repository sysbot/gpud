package config

import (
	"context"
	"fmt"
	"net"
	stdos "os"
	"path/filepath"
	"runtime"
	"time"

	nvidia_clock "github.com/leptonai/gpud/components/accelerator/nvidia/clock"
	nvidia_clockspeed "github.com/leptonai/gpud/components/accelerator/nvidia/clock-speed"
	nvidia_ecc "github.com/leptonai/gpud/components/accelerator/nvidia/ecc"
	nvidia_error "github.com/leptonai/gpud/components/accelerator/nvidia/error"
	nvidia_error_sxid "github.com/leptonai/gpud/components/accelerator/nvidia/error/sxid"
	nvidia_error_xid "github.com/leptonai/gpud/components/accelerator/nvidia/error/xid"
	nvidia_fabric_manager "github.com/leptonai/gpud/components/accelerator/nvidia/fabric-manager"
	nvidia_infiniband "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
	nvidia_info "github.com/leptonai/gpud/components/accelerator/nvidia/info"
	nvidia_memory "github.com/leptonai/gpud/components/accelerator/nvidia/memory"
	nvidia_nvlink "github.com/leptonai/gpud/components/accelerator/nvidia/nvlink"
	nvidia_peermem "github.com/leptonai/gpud/components/accelerator/nvidia/peermem"
	nvidia_power "github.com/leptonai/gpud/components/accelerator/nvidia/power"
	nvidia_processes "github.com/leptonai/gpud/components/accelerator/nvidia/processes"
	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	nvidia_temperature "github.com/leptonai/gpud/components/accelerator/nvidia/temperature"
	nvidia_utilization "github.com/leptonai/gpud/components/accelerator/nvidia/utilization"
	containerd_pod "github.com/leptonai/gpud/components/containerd/pod"
	"github.com/leptonai/gpud/components/cpu"
	"github.com/leptonai/gpud/components/disk"
	"github.com/leptonai/gpud/components/dmesg"
	docker_container "github.com/leptonai/gpud/components/docker/container"
	"github.com/leptonai/gpud/components/fd"
	"github.com/leptonai/gpud/components/info"
	k8s_pod "github.com/leptonai/gpud/components/k8s/pod"
	"github.com/leptonai/gpud/components/memory"
	"github.com/leptonai/gpud/components/os"
	power_supply "github.com/leptonai/gpud/components/power-supply"
	query_config "github.com/leptonai/gpud/components/query/config"
	component_systemd "github.com/leptonai/gpud/components/systemd"
	"github.com/leptonai/gpud/components/tailscale"
	"github.com/leptonai/gpud/log"
	pkd_systemd "github.com/leptonai/gpud/pkg/systemd"
	"github.com/leptonai/gpud/systemd"
	"github.com/leptonai/gpud/version"

	"github.com/mitchellh/go-homedir"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DefaultAPIVersion = "v1"
	DefaultGPUdPort   = 15132
)

var (
	DefaultRefreshPeriod   = metav1.Duration{Duration: time.Minute}
	DefaultRetentionPeriod = metav1.Duration{Duration: 30 * time.Minute}
)

func DefaultConfig(ctx context.Context) (*Config, error) {
	cfg := &Config{
		APIVersion: DefaultAPIVersion,

		Annotations: map[string]string{
			"version": version.Version,
		},

		Address: fmt.Sprintf(":%d", DefaultGPUdPort),

		// default components that work both in mac/linux
		Components: map[string]any{
			cpu.Name:    nil,
			disk.Name:   disk.DefaultConfig(),
			fd.Name:     nil,
			info.Name:   nil,
			memory.Name: nil,
			os.Name:     nil,
		},

		RetentionPeriod: DefaultRetentionPeriod,
		Pprof:           false,

		Web: &Web{
			Enable:        true,
			Admin:         false,
			RefreshPeriod: DefaultRefreshPeriod,
			SincePeriod:   DefaultRetentionPeriod,
		},
	}

	if runtime.GOOS == "linux" {
		containerdSocketExists := false
		containerdRunning := false

		if _, err := stdos.Stat(containerd_pod.DefaultSocketFile); err == nil {
			log.Logger.Debugw("containerd default socket file exists, containerd installed", "file", containerd_pod.DefaultSocketFile)
			containerdSocketExists = true
		} else {
			log.Logger.Debugw("containerd default socket file does not exist, skip containerd check", "file", containerd_pod.DefaultSocketFile, "error", err)
		}

		cctx, ccancel := context.WithTimeout(ctx, 5*time.Second)
		defer ccancel()
		if _, _, conn, err := containerd_pod.Connect(cctx, containerd_pod.DefaultContainerRuntimeEndpoint); err == nil {
			log.Logger.Debugw("containerd default cri endpoint open, containerd running", "endpoint", containerd_pod.DefaultContainerRuntimeEndpoint)
			containerdRunning = true
			_ = conn.Close()
		} else {
			log.Logger.Debugw("containerd default cri endpoint not open, skip containerd checking", "endpoint", containerd_pod.DefaultContainerRuntimeEndpoint, "error", err)
		}

		if containerdSocketExists && containerdRunning {
			log.Logger.Debugw("auto-detected containerd -- configuring containerd pod component")
			cfg.Components[containerd_pod.Name] = containerd_pod.Config{
				Query:    query_config.DefaultConfig(),
				Endpoint: containerd_pod.DefaultContainerRuntimeEndpoint,
			}
		}
	} else {
		log.Logger.Debugw("ignoring default containerd pod checking since it's not linux", "os", runtime.GOOS)
	}

	if runtime.GOOS == "linux" {
		// check if the TCP port is open/used
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", k8s_pod.DefaultKubeletReadOnlyPort), 3*time.Second)
		if err != nil {
			log.Logger.Debugw("tcp port is not open", "port", k8s_pod.DefaultKubeletReadOnlyPort, "error", err)
		} else {
			log.Logger.Debugw("tcp port is open", "port", k8s_pod.DefaultKubeletReadOnlyPort)
			conn.Close()

			kerr := k8s_pod.CheckKubeletReadOnlyPort(ctx, k8s_pod.DefaultKubeletReadOnlyPort)
			// check
			if kerr != nil {
				log.Logger.Debugw("kubelet readonly port is not open", "port", k8s_pod.DefaultKubeletReadOnlyPort, "error", kerr)
			} else {
				log.Logger.Debugw("auto-detected kubelet readonly port -- configuring k8s pod components", "port", k8s_pod.DefaultKubeletReadOnlyPort)

				// "k8s_pod" requires kubelet read-only port
				// assume if kubelet is running, it opens the most common read-only port 10255
				cfg.Components[k8s_pod.Name] = k8s_pod.Config{
					Query: query_config.DefaultConfig(),
					Port:  k8s_pod.DefaultKubeletReadOnlyPort,
				}
			}
		}
	} else {
		log.Logger.Debugw("ignoring default kubelet checking since it's not linux", "os", runtime.GOOS)
	}

	if docker_container.IsDockerRunning() {
		log.Logger.Debugw("auto-detected docker -- configuring docker container component")
		cfg.Components[docker_container.Name] = nil
	}

	if _, err := stdos.Stat(power_supply.DefaultBatteryCapacityFile); err == nil {
		cfg.Components[power_supply.Name] = nil
	}

	if runtime.GOOS == "linux" {
		if dmesg.DmesgExists() {
			log.Logger.Debugw("auto-detected dmesg -- configuring dmesg component")
			cfg.Components[dmesg.Name] = dmesg.DefaultConfig()
		}
	} else {
		log.Logger.Debugw("auto-detect dmesg not supported -- skipping", "os", runtime.GOOS)
	}

	if runtime.GOOS == "linux" {
		if pkd_systemd.SystemdExists() && pkd_systemd.SystemctlExists() {
			if err := systemd.CreateDefaultEnvFile(); err != nil {
				log.Logger.Debugw("failed to create default systemd env file", "error", err)
			}

			log.Logger.Debugw("auto-detected systemd -- configuring systemd component")

			systemdCfg := component_systemd.DefaultConfig()

			if active, err := pkd_systemd.IsActive("kubelet"); err == nil && active {
				systemdCfg.Units = append(systemdCfg.Units, "kubelet")
			}

			if active, err := pkd_systemd.IsActive("docker"); err == nil && active {
				systemdCfg.Units = append(systemdCfg.Units, "docker")
			}

			if active, err := pkd_systemd.IsActive("tailscaled"); err == nil && active {
				systemdCfg.Units = append(systemdCfg.Units, "tailscaled")
			}

			cfg.Components[component_systemd.Name] = systemdCfg
		}
	} else {
		log.Logger.Debugw("auto-detect systemd not supported -- skipping", "os", runtime.GOOS)
	}

	if runtime.GOOS == "linux" {
		if tailscale.TailscaleExists() {
			log.Logger.Debugw("auto-detected tailscale -- configuring tailscale component")
			cfg.Components[tailscale.Name] = nil
		}
	} else {
		log.Logger.Debugw("auto-detect tailscale not supported -- skipping", "os", runtime.GOOS)
	}

	if runtime.GOOS == "linux" {
		if nvidia_query.SMIExists() {
			log.Logger.Debugw("auto-detected nvidia -- configuring nvidia components")

			cfg.Components[nvidia_clock.Name] = nil
			cfg.Components[nvidia_ecc.Name] = nil
			cfg.Components[nvidia_error.Name] = nil
			if _, ok := cfg.Components[dmesg.Name]; ok {
				cfg.Components[nvidia_error_xid.Name] = nil
				cfg.Components[nvidia_error_sxid.Name] = nil
			}
			cfg.Components[nvidia_info.Name] = nil

			cfg.Components[nvidia_clockspeed.Name] = nil
			cfg.Components[nvidia_memory.Name] = nil
			cfg.Components[nvidia_nvlink.Name] = nil
			cfg.Components[nvidia_power.Name] = nil
			cfg.Components[nvidia_temperature.Name] = nil
			cfg.Components[nvidia_utilization.Name] = nil
			cfg.Components[nvidia_processes.Name] = nil

			// optional
			cfg.Components[nvidia_fabric_manager.Name] = nil
			cfg.Components[nvidia_infiniband.Name] = nil
			cfg.Components[nvidia_peermem.Name] = nil
		}
	} else {
		log.Logger.Debugw("auto-detect nvidia not supported -- skipping", "os", runtime.GOOS)
	}

	if cfg.State == "" {
		var err error
		cfg.State, err = DefaultStateFile()
		if err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

const defaultVarLib = "/var/lib/gpud"

func setupDefaultDir() (string, error) {
	asRoot := stdos.Geteuid() == 0 // running as root

	d := defaultVarLib
	_, err := stdos.Stat("/var/lib")
	if !asRoot || stdos.IsNotExist(err) {
		homeDir, err := homedir.Dir()
		if err != nil {
			return "", err
		}
		d = filepath.Join(homeDir, ".gpud")
	}

	if _, err := stdos.Stat(d); stdos.IsNotExist(err) {
		if err = stdos.MkdirAll(d, 0755); err != nil {
			return "", err
		}
	}
	return d, nil
}

func DefaultConfigFile() (string, error) {
	dir, err := setupDefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gpud.yaml"), nil
}

func DefaultStateFile() (string, error) {
	dir, err := setupDefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gpud.state"), nil
}

func DefaultFifoFile() (string, error) {
	f, err := setupDefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(f, "gpud.fifo"), nil
}
