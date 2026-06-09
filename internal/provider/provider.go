package provider

import (
	"fmt"

	"github.com/DeprecatedLuar/agentctl/internal/config"
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
func NewProvider(cfg *config.AgentConfig, agentFolder string) (Provider, error) {
	switch cfg.Provider {
	case "openai":
		return NewOpenAIProvider(cfg, agentFolder)
	case "openrouter":
		return NewOpenRouterProvider(cfg, agentFolder)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}
}
