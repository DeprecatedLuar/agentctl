package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/DeprecatedLuar/agentctl/internal/resolution"
	"github.com/joho/godotenv"
)

const (
	// File names
	agentConfigFile = "config/agent.toml"
	envFile         = ".env"

	// Provider names (duplicated from provider package to avoid circular dependency)
	providerOpenAI     = "openai"
	providerOpenRouter = "openrouter"

	// Valid gateway names
	gatewayCLI      = "cli"
	gatewayTelegram = "telegram"

	// API key environment variables
	envOpenAI     = "OPENAI_API_KEY"
	envOpenRouter = "OPENROUTER_API_KEY"
	envTelegram   = "TELEGRAM_BOT_TOKEN"

	// Reasoning modes ([advanced] reasoning)
	ReasoningNone  = "none"
	ReasoningTools = "tools"
	ReasoningAll   = "all" // reserved: aliases to ReasoningTools until user-facing display exists

	// Valid reasoning_effort values (OpenRouter/Anthropic/Gemini/OpenAI o-series)
	effortMinimal = "minimal"
	effortLow     = "low"
	effortMedium  = "medium"
	effortHigh    = "high"
	effortMax     = "max"
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

// HasBlockingError reports whether any issue in the slice is severe enough to
// block execution, as opposed to a non-fatal warning. This is the single
// place that decides what counts as blocking vs. advisory.
func HasBlockingError(issues []ValidationIssue) bool {
	for _, issue := range issues {
		if issue.Type == IssueError {
			return true
		}
	}
	return false
}

type AgentConfig struct {
	Agent       AgentSection      `toml:"agent"`
	Access      AccessConfig      `toml:"access"`
	Memory      MemoryConfig      `toml:"memory"`
	Audio       *AudioConfig      `toml:"audio"`       // Optional
	Advanced    AdvancedConfig    `toml:"advanced"`    // Optional
	Environment map[string]string `toml:"environment"` // Optional
}

type AgentSection struct {
	Provider string      `toml:"provider"`
	Model    string      `toml:"model"`
	Tools    []string    `toml:"tools"`
	Logging  interface{} `toml:"logging"` // false | true | "debug"
}

// Overrides holds per-call config overrides (flag/env driven), applied on
// top of a freshly loaded AgentConfig so they survive hot-reload without
// being persisted to agent.toml.
type Overrides struct {
	Model    string
	Provider string
}

// Apply overwrites cfg's model/provider with any non-empty override values.
func (o Overrides) Apply(cfg *AgentConfig) {
	if o.Model != "" {
		cfg.Agent.Model = o.Model
	}
	if o.Provider != "" {
		cfg.Agent.Provider = o.Provider
	}
}

type MemoryConfig struct {
	MaxMessages int `toml:"max_messages"`
}

// HistoryEnabled reports whether session history should be loaded and
// persisted at all. A non-positive MaxMessages means memory is off.
func (m MemoryConfig) HistoryEnabled() bool {
	return m.MaxMessages > 0
}

type AudioConfig struct {
	Provider string `toml:"provider"` // "whisper" or http/https URL
	Model    string `toml:"model"`
}

type AdvancedConfig struct {
	Reasoning       string `toml:"reasoning"`        // none | tools | all (all reserved, aliases to tools)
	ReasoningEffort string `toml:"reasoning_effort"` // minimal | low | medium | high | max
}

// ReasoningMode normalizes the configured reasoning mode: empty defaults to
// ReasoningTools (on by default), and the reserved "all" value aliases to
// ReasoningTools until user-facing reasoning display exists.
func (a AdvancedConfig) ReasoningMode() string {
	switch a.Reasoning {
	case "", ReasoningTools, ReasoningAll:
		return ReasoningTools
	case ReasoningNone:
		return ReasoningNone
	default:
		return a.Reasoning // invalid value surfaced by LoadAgent validation
	}
}

// ReasoningEnabled reports whether reasoning should be requested from the
// provider and preserved across tool-call turns.
func (a AdvancedConfig) ReasoningEnabled() bool {
	return a.ReasoningMode() == ReasoningTools
}

type AccessConfig struct {
	AllowByDefault bool `toml:"allow_by_default"` // Default access policy for new contacts
	// Gateways lists daemon-hosted gateways (cli is a valid access identity
	// but never a daemon-hosted gateway - see assembleOrchestrator). FROZEN
	// on-disk key: every existing agent.toml still says `interfaces = [...]`.
	Gateways []string `toml:"interfaces"`
}

// LoggingEnabled returns true if file logging is enabled (any value except false)
func (a *AgentSection) LoggingEnabled() bool {
	if a.Logging == nil {
		return true // Default to enabled
	}
	if b, ok := a.Logging.(bool); ok {
		return b
	}
	// String values ("debug") always mean enabled
	return true
}

// IsDebugMode returns true if logging is set to "debug"
func (a *AgentSection) IsDebugMode() bool {
	if s, ok := a.Logging.(string); ok {
		return s == "debug"
	}
	return false
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

	// Process [environment] values through directive/variable resolution
	if cfg.Environment != nil {
		ctx := resolution.Context{
			AgentPath: agentPath,
			AgentName: filepath.Base(agentPath),
			// All other fields empty (no user/session context at config load)
		}

		for key, value := range cfg.Environment {
			processed, err := resolution.Process(value, ctx)
			if err != nil {
				issues = append(issues, ValidationIssue{
					Type:    IssueError,
					Message: fmt.Sprintf("%s: [environment] %s: %v", agentConfigFile, key, err),
				})
				continue
			}
			cfg.Environment[key] = processed
		}
	}

	// Validate required fields
	if cfg.Agent.Provider == "" {
		issues = append(issues, ValidationIssue{
			Type:    IssueError,
			Message: fmt.Sprintf("%s: provider field is required", agentConfigFile),
		})
	}
	if cfg.Agent.Model == "" {
		issues = append(issues, ValidationIssue{
			Type:    IssueError,
			Message: fmt.Sprintf("%s: model field is required", agentConfigFile),
		})
	}

	// Validate [advanced] reasoning settings
	switch cfg.Advanced.Reasoning {
	case "", ReasoningNone, ReasoningTools, ReasoningAll:
	default:
		issues = append(issues, ValidationIssue{
			Type:    IssueError,
			Message: fmt.Sprintf("%s: [advanced] reasoning must be 'none', 'tools', or 'all' (got %q)", agentConfigFile, cfg.Advanced.Reasoning),
		})
	}
	switch cfg.Advanced.ReasoningEffort {
	case "", effortMinimal, effortLow, effortMedium, effortHigh, effortMax:
	default:
		issues = append(issues, ValidationIssue{
			Type:    IssueError,
			Message: fmt.Sprintf("%s: [advanced] reasoning_effort must be one of minimal, low, medium, high, max (got %q)", agentConfigFile, cfg.Advanced.ReasoningEffort),
		})
	}

	// Validate gateways
	for _, gateway := range cfg.Access.Gateways {
		if gateway != gatewayCLI && gateway != gatewayTelegram {
			issues = append(issues, ValidationIssue{
				Type:    IssueError,
				Message: fmt.Sprintf("%s: invalid gateway '%s' (valid: cli, telegram)", agentConfigFile, gateway),
			})
		}
	}

	// Check API keys for configured provider
	envPath := filepath.Join(agentPath, envFile)
	_ = godotenv.Load(envPath)

	if cfg.Agent.Provider != "" && !strings.HasPrefix(cfg.Agent.Provider, "http") {
		// Named provider - API key is required
		switch cfg.Agent.Provider {
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

	// Check Telegram bot token if the telegram gateway is enabled
	for _, gateway := range cfg.Access.Gateways {
		if gateway == gatewayTelegram {
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
