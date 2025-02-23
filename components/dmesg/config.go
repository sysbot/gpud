package dmesg

import (
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"

	query_config "github.com/leptonai/gpud/components/query/config"
	query_log_config "github.com/leptonai/gpud/components/query/log/config"
)

type Config struct {
	Log query_log_config.Config `json:"log"`
}

func ParseConfig(b any, db *sql.DB) (*Config, error) {
	raw, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	cfg := new(Config)
	err = json.Unmarshal(raw, cfg)
	if err != nil {
		return nil, err
	}

	if cfg.Log.Query.State != nil {
		cfg.Log.Query.State.DB = db
	}
	cfg.Log.DB = db

	return cfg, nil
}

func (cfg Config) Validate() error {
	return cfg.Log.Validate()
}

func DmesgExists() bool {
	p, err := exec.LookPath("dmesg")
	if err != nil {
		return false
	}
	return p != ""
}

const DefaultDmesgFile = "/var/log/dmesg"

func DefaultConfig() Config {
	scanCommands := [][]string{
		{"cat", DefaultDmesgFile},
		{"dmesg", "--ctime", "--nopager", "--buffer-size", "163920", "--since", "'1 hour ago'"},
	}
	if _, err := os.Stat(DefaultDmesgFile); os.IsNotExist(err) {
		scanCommands = [][]string{
			{"dmesg", "--ctime", "--nopager", "--buffer-size", "163920", "--since", "'1 hour ago'"},
		}
	}

	cfg := Config{
		Log: query_log_config.Config{
			Query:      query_config.DefaultConfig(),
			BufferSize: query_log_config.DefaultBufferSize,

			Commands: [][]string{
				{"dmesg", "--ctime", "--nopager", "--buffer-size", "163920", "-w"},
				{"dmesg", "--ctime", "--nopager", "--buffer-size", "163920", "-W"},
			},

			Scan: &query_log_config.Scan{
				Commands:    scanCommands,
				LinesToTail: 10000,
			},
		},
	}
	cfg.Log.SelectFilters = append(cfg.Log.SelectFilters, defaultFilters...)

	return cfg
}
