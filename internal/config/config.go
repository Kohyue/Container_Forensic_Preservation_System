package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	// configPath is the file this config was loaded from; used by Save().
	// Unexported so it is not marshalled into YAML/JSON output.
	configPath string

	Agent      AgentConfig      `yaml:"agent"`
	Detector   DetectorConfig   `yaml:"detector"`
	Collector  CollectorConfig  `yaml:"collector"`
	Repository RepositoryConfig `yaml:"repository"`
	Audit      AuditConfig      `yaml:"audit"`
	Web        WebConfig        `yaml:"web"`
	Schedule   ScheduleConfig   `yaml:"schedule"`
}

type WebConfig struct {
	// Address the dashboard web server listens on
	Addr string `yaml:"addr"` // e.g. ":8080"
	// Path to the Falco rules file (shown in Settings UI)
	RulesPath string `yaml:"rules_path"`
}

type AgentConfig struct {
	NodeName string `yaml:"node_name"`
}

type DetectorConfig struct {
	// Webhook server that receives Falco alerts
	ListenAddr string `yaml:"listen_addr"` // e.g. ":8765"
	// Minimum Falco priority to act on
	MinPriority string `yaml:"min_priority"`
}

type CollectorConfig struct {
	// Max time to spend collecting evidence for one alert
	TimeoutSeconds int `yaml:"timeout_seconds"`
	// Whether to collect each artifact type
	CollectProcess  bool `yaml:"collect_process"`
	CollectLogs     bool `yaml:"collect_logs"`
	CollectNetwork  bool `yaml:"collect_network"`
	CollectMetadata bool `yaml:"collect_metadata"`
	// Max log lines to capture per container
	MaxLogLines int `yaml:"max_log_lines"`
}

type RepositoryConfig struct {
	// Base directory for storing evidence packages
	BasePath string `yaml:"base_path"`
	// Max total size in MB before oldest evidence is pruned (0 = no limit)
	MaxSizeMB int `yaml:"max_size_mb"`
}

type AuditConfig struct {
	// Path for the agent audit log
	LogPath string `yaml:"log_path"`
}

// ScheduleConfig controls whether captures are triggered immediately on each
// alert (Enabled = false) or batched and processed at a regular interval
// (Enabled = true).
type ScheduleConfig struct {
	Enabled         bool `yaml:"enabled"`
	IntervalSeconds int  `yaml:"interval_seconds"`
}

func (c *CollectorConfig) Timeout() time.Duration {
	if c.TimeoutSeconds <= 0 {
		return 30 * time.Second
	}
	return time.Duration(c.TimeoutSeconds) * time.Second
}

// Interval returns the scheduled-capture interval, defaulting to 5 minutes.
func (sc *ScheduleConfig) Interval() time.Duration {
	if sc.IntervalSeconds <= 0 {
		return 5 * time.Minute
	}
	return time.Duration(sc.IntervalSeconds) * time.Second
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	cfg := defaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.configPath = path
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return cfg, nil
}

// Save writes the current config back to the file it was loaded from.
// Note: YAML comments in the original file are not preserved.
func (c *Config) Save() error {
	if c.configPath == "" {
		return fmt.Errorf("config path not set")
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(c.configPath, data, 0o640)
}

func defaultConfig() *Config {
	return &Config{
		Detector: DetectorConfig{
			ListenAddr:  ":8765",
			MinPriority: "warning",
		},
		Collector: CollectorConfig{
			TimeoutSeconds:  30,
			CollectProcess:  true,
			CollectLogs:     true,
			CollectNetwork:  true,
			CollectMetadata: true,
			MaxLogLines:     1000,
		},
		Repository: RepositoryConfig{
			BasePath:  "/var/forensics/evidence",
			MaxSizeMB: 0,
		},
		Audit: AuditConfig{
			LogPath: "/var/forensics/audit.log",
		},
		Web: WebConfig{
			Addr:      ":8080",
			RulesPath: "/etc/forensic-agent/falco-rules.yaml",
		},
		Schedule: ScheduleConfig{
			Enabled:         false,
			IntervalSeconds: 300,
		},
	}
}

func (c *Config) validate() error {
	if c.Detector.ListenAddr == "" {
		return fmt.Errorf("detector.listen_addr is required")
	}
	if c.Repository.BasePath == "" {
		return fmt.Errorf("repository.base_path is required")
	}
	return nil
}
