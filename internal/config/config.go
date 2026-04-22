package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	defaultExtensions = []string{".mkv", ".mp4", ".m2ts", ".ts"}
	defaultRcloneArgs = []string{
		"--drive-chunk-size=256M",
		"--checkers=5",
		"--transfers=5",
		"--drive-stop-on-upload-limit",
		"--stats=1s",
		"--stats-one-line",
		"-v",
	}
)

type Config struct {
	PollInterval       time.Duration `yaml:"poll_interval"`
	StableDuration     time.Duration `yaml:"stable_duration"`
	RetryInterval      time.Duration `yaml:"retry_interval"`
	MaxParallelUploads int           `yaml:"max_parallel_uploads"`
	Extensions         []string      `yaml:"extensions"`
	RcloneArgs         []string      `yaml:"rclone_args"`
	Jobs               []JobConfig   `yaml:"jobs"`
}

type JobConfig struct {
	Name         string `yaml:"name"`
	SourceDir    string `yaml:"source_dir"`
	LinkDir      string `yaml:"link_dir"`
	RcloneRemote string `yaml:"rclone_remote"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	applyDefaults(&cfg)
	normalizeConfig(&cfg)

	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = time.Second
	}
	if cfg.StableDuration == 0 {
		cfg.StableDuration = time.Minute
	}
	if cfg.RetryInterval == 0 {
		cfg.RetryInterval = 10 * time.Minute
	}
	if cfg.MaxParallelUploads == 0 {
		cfg.MaxParallelUploads = 5
	}
	if len(cfg.Extensions) == 0 {
		cfg.Extensions = append([]string(nil), defaultExtensions...)
	}
	if len(cfg.RcloneArgs) == 0 {
		cfg.RcloneArgs = append([]string(nil), defaultRcloneArgs...)
	}
}

func normalizeConfig(cfg *Config) {
	for i := range cfg.Extensions {
		cfg.Extensions[i] = strings.ToLower(cfg.Extensions[i])
	}
}

func validateConfig(cfg *Config) error {
	if len(cfg.Jobs) == 0 {
		return errors.New("config must include at least one job")
	}

	sourceDirs := make(map[string]struct{}, len(cfg.Jobs))
	linkDirs := make(map[string]struct{}, len(cfg.Jobs))

	for _, job := range cfg.Jobs {
		if strings.TrimSpace(job.Name) == "" {
			return errors.New("job name is required")
		}
		if strings.TrimSpace(job.SourceDir) == "" {
			return errors.New("job source_dir is required")
		}
		if strings.TrimSpace(job.LinkDir) == "" {
			return errors.New("job link_dir is required")
		}
		if strings.TrimSpace(job.RcloneRemote) == "" {
			return errors.New("job rclone_remote is required")
		}
		if _, ok := sourceDirs[job.SourceDir]; ok {
			return fmt.Errorf("duplicate source_dir: %s", job.SourceDir)
		}
		if _, ok := linkDirs[job.LinkDir]; ok {
			return fmt.Errorf("duplicate link_dir: %s", job.LinkDir)
		}
		sourceDirs[job.SourceDir] = struct{}{}
		linkDirs[job.LinkDir] = struct{}{}
	}

	return nil
}
