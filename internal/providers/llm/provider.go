package llm

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/DeprecatedLuar/agentctl/internal/config"
)

const (
	// Provider names
	ProviderOpenAI     = "openai"
	ProviderOpenRouter = "openrouter"

	// Message roles
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Message represents a single message in the conversation
type Message struct {
	Role    string
	Content string
}

// ToolCall represents a tool invocation from the AI
type ToolCall struct {
	ID       string
	Name     string
	Args     map[string]interface{}
}

// Provider handles communication with AI providers
type Provider interface {
	// SendMessages sends messages and returns the assistant's response
	// If tool calls are requested, returns them alongside the response
	SendMessages(messages []Message, tools []config.ToolConfig) (response string, toolCalls []ToolCall, err error)
}

// NewProvider creates a provider based on the config
func NewProvider(cfg *config.AgentConfig, agentFolder string, logger *slog.Logger) (Provider, error) {
	// Check if provider is a custom endpoint (http/https URL)
	if strings.HasPrefix(cfg.Provider, "http://") || strings.HasPrefix(cfg.Provider, "https://") {
		return NewGenericProvider(cfg, agentFolder, logger)
	}

	// Named providers
	switch cfg.Provider {
	case ProviderOpenAI:
		return NewOpenAIProvider(cfg, agentFolder, logger)
	case ProviderOpenRouter:
		return NewOpenRouterProvider(cfg, agentFolder, logger)
	default:
		return nil, fmt.Errorf("unsupported provider: %s (must be 'openai', 'openrouter', or an http/https URL)", cfg.Provider)
	}
}
