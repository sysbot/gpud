package nvml

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/dustin/go-humanize"
	"github.com/leptonai/gpud/log"
	"github.com/shirou/gopsutil/v4/process"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// Processes represents the current clock events from the nvmlDeviceGetCurrentClocksEventReasons API.
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g7e505374454a0d4fc7339b6c885656d6
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1ga115e41a14b747cb334a0e7b49ae1941
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlClocksEventReasons.html#group__nvmlClocksEventReasons
type Processes struct {
	// Represents the GPU UUID.
	UUID string `json:"uuid"`

	// A list of running processes.
	RunningProcesses []Process `json:"running_processes"`
}

type Process struct {
	PID                         uint32      `json:"pid"`
	Status                      []string    `json:"status,omitempty"`
	CmdArgs                     []string    `json:"cmd_args,omitempty"`
	CreateTime                  metav1.Time `json:"create_time,omitempty"`
	GPUUsedPercent              uint32      `json:"gpu_used_percent,omitempty"`
	GPUUsedMemoryBytes          uint64      `json:"gpu_used_memory_bytes,omitempty"`
	GPUUsedMemoryBytesHumanized string      `json:"gpu_used_memory_bytes_humanized,omitempty"`
}

func (procs *Processes) JSON() ([]byte, error) {
	return json.Marshal(procs)
}

func (procs *Processes) YAML() ([]byte, error) {
	return yaml.Marshal(procs)
}

func GetProcesses(uuid string, dev device.Device) (Processes, error) {
	procs := Processes{
		UUID: uuid,
	}

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g34afcba3d32066db223265aa022a6b80
	computeProcs, ret := dev.GetComputeRunningProcesses()
	if ret != nvml.SUCCESS {
		return Processes{}, fmt.Errorf("failed to get device compute processes: %v", nvml.ErrorString(ret))
	}

	for _, proc := range computeProcs {
		procObject, err := process.NewProcess(int32(proc.Pid))
		if err != nil {
			// ref. process does not exist
			if errors.Is(err, process.ErrorProcessNotRunning) {
				log.Logger.Debugw("process not running -- skipping", "pid", proc.Pid, "error", err)
				continue
			}
			return Processes{}, fmt.Errorf("failed to get process %d: %v", proc.Pid, err)
		}

		args, err := procObject.CmdlineSlice()
		if err != nil {
			return Processes{}, fmt.Errorf("failed to get process %d args: %v", proc.Pid, err)
		}
		createTimeUnixMS, err := procObject.CreateTime()
		if err != nil {
			return Processes{}, fmt.Errorf("failed to get process %d create time: %v", proc.Pid, err)
		}
		createTime := metav1.Unix(createTimeUnixMS/1000, 0)

		// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1gb0ea5236f5e69e63bf53684a11c233bd
		memUtil := uint32(0)
		utils, ret := dev.GetProcessUtilization(uint64(proc.Pid))
		if ret != nvml.SUCCESS {
			es := nvml.ErrorString(ret)

			// e.g., Not Found
			if strings.Contains(strings.ToLower(es), "not found") {
				continue
			}
			return Processes{}, fmt.Errorf("failed to get process %d utilization: %v", proc.Pid, es)
		}
		if len(utils) > 0 {
			// sort by last seen timestamp, so that first is the latest
			sort.Slice(utils, func(i, j int) bool {
				return utils[i].TimeStamp > utils[j].TimeStamp
			})

			// ref. https://docs.nvidia.com/deploy/nvml-api/structnvmlProcessUtilizationSample__t.html#structnvmlProcessUtilizationSample__t
			memUtil = utils[0].MemUtil
		}

		status, err := procObject.Status()
		if err != nil {
			// e.g., Not Found
			if strings.Contains(strings.ToLower(err.Error()), "not found") {
				continue
			}
			return Processes{}, fmt.Errorf("failed to get process %d status: %v", proc.Pid, err)
		}

		procs.RunningProcesses = append(procs.RunningProcesses, Process{
			PID:        proc.Pid,
			Status:     status,
			CmdArgs:    args,
			CreateTime: createTime,

			GPUUsedPercent: memUtil,

			// "Amount of used GPU memory in bytes."
			// ref. https://docs.nvidia.com/deploy/nvml-api/structnvmlProcessInfo__t.html#structnvmlProcessInfo__t
			GPUUsedMemoryBytes:          proc.UsedGpuMemory,
			GPUUsedMemoryBytesHumanized: humanize.Bytes(proc.UsedGpuMemory),
		})
	}

	return procs, nil
}
