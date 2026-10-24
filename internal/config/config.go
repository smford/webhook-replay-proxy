package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents application configuration.
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Storage  StorageConfig  `yaml:"storage"`
	Upstream UpstreamConfig `yaml:"upstream"`
}

// ServerConfig defines HTTP server options.
type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

// StorageConfig defines webhook event storage limits.
type StorageConfig struct {
	MaxEvents int `yaml:"max_events"`
}

// UpstreamConfig defines target forwarding configuration.
type UpstreamConfig struct {
	DefaultTargetURL string `yaml:"default_target_url"`
}

// DefaultConfig returns default configuration parameters.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
		},
		Storage: StorageConfig{
			MaxEvents: 1000,
		},
	}
}

// Load reads and parses a YAML configuration file.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()
	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config YAML: %w", err)
	}

	return cfg, nil
}
