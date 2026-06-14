package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/joho/godotenv"
)

const (
	// File names
	agentConfigFile = "config/agent.toml"
	envFile         = ".env"

	// Provider names (duplicated from provider package to avoid circular dependency)
	providerOpenAI     = "openai"
	providerOpenRouter = "openrouter"

	// Valid interface names
	interfaceCLI      = "cli"
	interfaceTelegram = "telegram"

	// API key environment variables
	envOpenAI     = "OPENAI_API_KEY"
	envOpenRouter = "OPENROUTER_API_KEY"
	envTelegram   = "TELEGRAM_BOT_TOKEN"
)

// ValidationIssueType represents the severity of a validation issue
type ValidationIssueType string

const (
	IssueError   ValidationIssueType = "error"
	IssueWarning ValidationIssueType = "warn"
)

// ValidationIssue represents a configuration validation problem
type ValidationIssue struct {
	Type    ValidationIssueType
	Message string
}

type AgentConfig struct {
	Provider   string       `toml:"provider"`
	Model      string       `toml:"model"`
	Tools      []string     `toml:"tools"`
	Interfaces []string     `toml:"interfaces"`
	Memory     MemoryConfig `toml:"memory"`
	Audio      *AudioConfig `toml:"audio"`      // Optional
	Logging    *bool        `toml:"logging"`    // Default: true if nil
	DebugCalls *bool        `toml:"debug_calls"` // Default: false if nil, writes full request/response to .data/debug-calls/
}

type MemoryConfig struct {
	MaxMessages int `toml:"max_messages"`
}

type AudioConfig struct {
	Provider string `toml:"provider"` // "whisper" or http/https URL
	Model    string `toml:"model"`
}

func LoadAgent(agentPath string) (*AgentConfig, []ValidationIssue) {
	var issues []ValidationIssue

	configPath := filepath.Join(agentPath, agentConfigFile)

	data, err := os.ReadFile(configPath)
	if err != nil {
		issues = append(issues, ValidationIssue{
			Type:    IssueError,
			Message: fmt.Sprintf("%s: %v", agentConfigFile, err),
		})
		return nil, issues
	}

	var cfg AgentConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		issues = append(issues, ValidationIssue{
			Type:    IssueError,
			Message: fmt.Sprintf("%s: failed to parse TOML: %v", agentConfigFile, err),
		})
		return nil, issues
	}

	// Validate required fields
	if cfg.Provider == "" {
		issues = append(issues, ValidationIssue{
			Type:    IssueError,
			Message: fmt.Sprintf("%s: provider field is required", agentConfigFile),
		})
	}
	if cfg.Model == "" {
		issues = append(issues, ValidationIssue{
			Type:    IssueError,
			Message: fmt.Sprintf("%s: model field is required", agentConfigFile),
		})
	}

	// Validate interfaces
	for _, iface := range cfg.Interfaces {
		if iface != interfaceCLI && iface != interfaceTelegram {
			issues = append(issues, ValidationIssue{
				Type:    IssueError,
				Message: fmt.Sprintf("%s: invalid interface '%s' (valid: cli, telegram)", agentConfigFile, iface),
			})
		}
	}

	// Check API keys for configured provider
	envPath := filepath.Join(agentPath, envFile)
	_ = godotenv.Load(envPath)

	if cfg.Provider != "" && !strings.HasPrefix(cfg.Provider, "http") {
		// Named provider - API key is required
		switch cfg.Provider {
		case providerOpenAI:
			if os.Getenv(envOpenAI) == "" {
				issues = append(issues, ValidationIssue{
					Type:    IssueError,
					Message: fmt.Sprintf("%s not set in %s or environment", envOpenAI, envFile),
				})
			}
		case providerOpenRouter:
			if os.Getenv(envOpenRouter) == "" {
				issues = append(issues, ValidationIssue{
					Type:    IssueError,
					Message: fmt.Sprintf("%s not set in %s or environment", envOpenRouter, envFile),
				})
			}
		}
	}
	// HTTP endpoints: skip validation (unknown auth requirements)

	// Check Telegram bot token if telegram interface is enabled
	for _, iface := range cfg.Interfaces {
		if iface == interfaceTelegram {
			if os.Getenv(envTelegram) == "" {
				issues = append(issues, ValidationIssue{
					Type:    IssueError,
					Message: fmt.Sprintf("%s not set in %s or environment", envTelegram, envFile),
				})
			}
			break
		}
	}

	return &cfg, issues
}

// IsLoggingEnabled returns true if logging is enabled (default: true)
func (c *AgentConfig) IsLoggingEnabled() bool {
	if c.Logging == nil {
		return true // Default
	}
	return *c.Logging
}

func (c *AgentConfig) IsDebugCallsEnabled() bool {
	if c.DebugCalls == nil {
		return false // Default
	}
	return *c.DebugCalls
}
