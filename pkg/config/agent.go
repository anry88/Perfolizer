package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

const (
	DefaultAgentConfigPath = "config/agent.json"
	defaultListenHost      = "127.0.0.1"
	defaultPort            = 9090
	defaultPollSeconds     = 15
)

type AgentConfig struct {
	ListenHost        string `json:"listen_host"`
	Port              int    `json:"port"`
	UIPollIntervalSec int    `json:"ui_poll_interval_seconds"`
	UIConnectHost     string `json:"ui_connect_host,omitempty"`
}

func DefaultAgentConfig() AgentConfig {
	return AgentConfig{
		ListenHost:        defaultListenHost,
		Port:              defaultPort,
		UIPollIntervalSec: defaultPollSeconds,
	}
}

func ResolveAgentConfigPath() string {
	if fromEnv := os.Getenv("PERFOLIZER_AGENT_CONFIG"); fromEnv != "" {
		return fromEnv
	}
	return DefaultAgentConfigPath
}

func LoadAgentConfig(path string) (AgentConfig, error) {
	cfg := DefaultAgentConfig()

	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		return DefaultAgentConfig(), err
	}

	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func (c *AgentConfig) applyDefaults() {
	if c.ListenHost == "" {
		c.ListenHost = defaultListenHost
	}
	if c.Port == 0 {
		c.Port = defaultPort
	}
	if c.UIPollIntervalSec == 0 {
		c.UIPollIntervalSec = defaultPollSeconds
	}
}

func (c AgentConfig) Validate() error {
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Port)
	}
	if c.UIPollIntervalSec <= 0 {
		return fmt.Errorf("ui_poll_interval_seconds must be > 0")
	}
	return nil
}

func (c AgentConfig) ListenAddr() string {
	return fmt.Sprintf("%s:%d", c.ListenHost, c.Port)
}

func (c AgentConfig) UIHost() string {
	if c.UIConnectHost != "" {
		return c.UIConnectHost
	}
	if c.ListenHost == "" || c.ListenHost == "0.0.0.0" {
		return "127.0.0.1"
	}
	return c.ListenHost
}

func (c AgentConfig) BaseURL() string {
	return fmt.Sprintf("http://%s:%d", c.UIHost(), c.Port)
}
