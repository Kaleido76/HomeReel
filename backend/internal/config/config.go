package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server ServerConfig `yaml:"server"`
	Auth   AuthConfig   `yaml:"auth"`
	Media  MediaConfig  `yaml:"media"`
}

type ServerConfig struct {
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
	DataDir string `yaml:"data_dir"`
}

type AuthConfig struct {
	Password    string `yaml:"password"`
	SessionDays int    `yaml:"session_days"`
}

type MediaConfig struct {
	FFmpegPath       string `yaml:"ffmpeg_path"`
	FFprobePath      string `yaml:"ffprobe_path"`
	ProbeConcurrency int    `yaml:"probe_concurrency"`
}

func Default() Config {
	return Config{
		Server: ServerConfig{
			Host:    "0.0.0.0",
			Port:    8080,
			DataDir: "data",
		},
		Auth: AuthConfig{
			Password:    "",
			SessionDays: 30,
		},
		Media: MediaConfig{
			FFmpegPath:       "ffmpeg",
			FFprobePath:      "ffprobe",
			ProbeConcurrency: 2,
		},
	}
}

// Load reads the YAML config at path. When the file does not exist it writes a
// default config file (so the operator can edit it) and returns the defaults.
func Load(path string) (Config, error) {
	cfg := Default()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		data, err := yaml.Marshal(cfg)
		if err != nil {
			return cfg, err
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return cfg, fmt.Errorf("write default config: %w", err)
		}
		return cfg, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Server.DataDir == "" {
		cfg.Server.DataDir = filepath.FromSlash("data")
	}
	return cfg, nil
}
