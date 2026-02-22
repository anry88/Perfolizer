package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"perfolizer/pkg/config"
)

func TestDefaultAgentConfig(t *testing.T) {
	cfg := config.DefaultAgentConfig()
	if cfg.ListenHost != "127.0.0.1" {
		t.Fatalf("expected default listen host %q, got %q", "127.0.0.1", cfg.ListenHost)
	}
	if cfg.Port != 9090 {
		t.Fatalf("expected default port 9090, got %d", cfg.Port)
	}
	if cfg.UIPollIntervalSec != 5 {
		t.Fatalf("expected default poll interval 5, got %d", cfg.UIPollIntervalSec)
	}
}

func TestResolveAgentConfigPath(t *testing.T) {
	t.Setenv("PERFOLIZER_AGENT_CONFIG", "/tmp/custom-agent.json")
	if got := config.ResolveAgentConfigPath(); got != "/tmp/custom-agent.json" {
		t.Fatalf("expected env path, got %q", got)
	}

	t.Setenv("PERFOLIZER_AGENT_CONFIG", "")
	if got := config.ResolveAgentConfigPath(); got != config.DefaultAgentConfigPath {
		t.Fatalf("expected default config path %q, got %q", config.DefaultAgentConfigPath, got)
	}
}

func TestLoadAgentConfigMissingFileReturnsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	cfg, err := config.LoadAgentConfig(path)
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}

	defaults := config.DefaultAgentConfig()
	if cfg != defaults {
		t.Fatalf("expected defaults %#v, got %#v", defaults, cfg)
	}
}

func TestLoadAgentConfigInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("{broken"), 0o644); err != nil {
		t.Fatalf("failed to write bad config: %v", err)
	}

	cfg, err := config.LoadAgentConfig(path)
	if err == nil {
		t.Fatal("expected parse error")
	}

	defaults := config.DefaultAgentConfig()
	if cfg != defaults {
		t.Fatalf("expected defaults on parse error %#v, got %#v", defaults, cfg)
	}
}

func TestLoadAgentConfigAppliesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.json")
	if err := os.WriteFile(path, []byte(`{"port":19090}`), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := config.LoadAgentConfig(path)
	if err != nil {
		t.Fatalf("LoadAgentConfig returned error: %v", err)
	}

	if cfg.ListenHost != "127.0.0.1" {
		t.Fatalf("expected default listen host, got %q", cfg.ListenHost)
	}
	if cfg.Port != 19090 {
		t.Fatalf("expected configured port 19090, got %d", cfg.Port)
	}
	if cfg.UIPollIntervalSec != 5 {
		t.Fatalf("expected default poll interval 5, got %d", cfg.UIPollIntervalSec)
	}
}

func TestLoadAgentConfigValidationError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(path, []byte(`{"listen_host":"0.0.0.0","port":70000,"ui_poll_interval_seconds":1}`), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := config.LoadAgentConfig(path)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if cfg.Port != 70000 {
		t.Fatalf("expected loaded invalid port for diagnostics, got %d", cfg.Port)
	}
}

func TestAgentConfigDerivedAddresses(t *testing.T) {
	cfg := config.AgentConfig{ListenHost: "0.0.0.0", Port: 8080, UIPollIntervalSec: 10}
	if cfg.ListenAddr() != "0.0.0.0:8080" {
		t.Fatalf("unexpected listen addr: %q", cfg.ListenAddr())
	}
	if cfg.UIHost() != "127.0.0.1" {
		t.Fatalf("expected loopback UI host for wildcard listen host, got %q", cfg.UIHost())
	}
	if cfg.BaseURL() != "http://127.0.0.1:8080" {
		t.Fatalf("unexpected base URL: %q", cfg.BaseURL())
	}

	cfg.UIConnectHost = "agent.local"
	if cfg.UIHost() != "agent.local" {
		t.Fatalf("expected explicit UI host override, got %q", cfg.UIHost())
	}
	if cfg.BaseURL() != "http://agent.local:8080" {
		t.Fatalf("unexpected overridden base URL: %q", cfg.BaseURL())
	}
}
