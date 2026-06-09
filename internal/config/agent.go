package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const (
	// File names
	agentConfigFile = "agent.toml"

	// Provider names (duplicated from provider package to avoid circular dependency)
	providerOpenAI     = "openai"
	providerOpenRouter = "openrouter"
)

type AgentConfig struct {
	Provider   string       `toml:"provider"`
	Model      string       `toml:"model"`
	Tools      []string     `toml:"tools"`
	Interfaces []string     `toml:"interfaces"`
	Memory     MemoryConfig `toml:"memory"`
	Logging    *bool        `toml:"logging"` // Default: true if nil
}

type MemoryConfig struct {
	MaxMessages int `toml:"max_messages"`
}

func LoadAgent(agentPath string) (*AgentConfig, error) {
	configPath := filepath.Join(agentPath, agentConfigFile)

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", agentConfigFile, err)
	}

	var cfg AgentConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", agentConfigFile, err)
	}

	// Validation
	if cfg.Provider == "" {
		return nil, fmt.Errorf("provider is required in %s", agentConfigFile)
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("model is required in %s", agentConfigFile)
	}
	if cfg.Provider != providerOpenAI && cfg.Provider != providerOpenRouter {
		return nil, fmt.Errorf("provider must be '%s' or '%s', got '%s'", providerOpenAI, providerOpenRouter, cfg.Provider)
	}

	return &cfg, nil
}

// IsLoggingEnabled returns true if logging is enabled (default: true)
func (c *AgentConfig) IsLoggingEnabled() bool {
	if c.Logging == nil {
		return true // Default
	}
	return *c.Logging
}
