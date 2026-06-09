package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type AgentConfig struct {
	Provider string       `toml:"provider"`
	Model    string       `toml:"model"`
	Tools    []string     `toml:"tools"`
	Memory   MemoryConfig `toml:"memory"`
}

type MemoryConfig struct {
	MaxMessages int `toml:"max_messages"`
}

func LoadAgent(agentPath string) (*AgentConfig, error) {
	configPath := filepath.Join(agentPath, "agent.toml")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read agent.toml: %w", err)
	}

	var cfg AgentConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse agent.toml: %w", err)
	}

	// Validation
	if cfg.Provider == "" {
		return nil, fmt.Errorf("provider is required in agent.toml")
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("model is required in agent.toml")
	}
	if cfg.Provider != "openai" && cfg.Provider != "openrouter" {
		return nil, fmt.Errorf("provider must be 'openai' or 'openrouter', got '%s'", cfg.Provider)
	}

	return &cfg, nil
}
