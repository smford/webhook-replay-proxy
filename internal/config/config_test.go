package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Server.Port != 8080 {
		t.Fatalf("expected port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Fatalf("expected host 0.0.0.0, got %s", cfg.Server.Host)
	}
	if cfg.Storage.MaxEvents != 1000 {
		t.Fatalf("expected max events 1000, got %d", cfg.Storage.MaxEvents)
	}
}

func TestLoadNonExistent(t *testing.T) {
	cfg, err := Load("non-existent-file.yaml")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Fatalf("expected port 8080, got %d", cfg.Server.Port)
	}
}

func TestLoadEmpty(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("expected nil error for empty path, got %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Fatalf("expected port 8080, got %d", cfg.Server.Port)
	}
}

func TestLoadValidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `
server:
  host: "127.0.0.1"
  port: 9090
storage:
  max_events: 500
upstream:
  default_target_url: "https://example.com/hook"
`
	if err := os.WriteFile(cfgFile, []byte(yamlContent), 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected host 127.0.0.1, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Storage.MaxEvents != 500 {
		t.Errorf("expected max events 500, got %d", cfg.Storage.MaxEvents)
	}
	if cfg.Upstream.DefaultTargetURL != "https://example.com/hook" {
		t.Errorf("expected default_target_url, got %s", cfg.Upstream.DefaultTargetURL)
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "invalid.yaml")

	if err := os.WriteFile(cfgFile, []byte("invalid: [yaml: broken"), 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := Load(cfgFile)
	if err == nil {
		t.Fatalf("expected error parsing invalid YAML, got nil")
	}
}
