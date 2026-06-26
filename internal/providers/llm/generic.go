package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/DeprecatedLuar/agentctl/internal/config"
	"github.com/joho/godotenv"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const (
	// Generic provider configuration
	genericEnvFile = ".env"
	genericAPIKey  = "OPENAI_API_KEY"
)

// GenericProvider implements Provider for OpenAI-compatible endpoints
type GenericProvider struct {
	client   openai.Client
	model    string
	logger   *slog.Logger
	endpoint string
}

// NewGenericProvider creates a provider for OpenAI-compatible endpoints
func NewGenericProvider(cfg *config.AgentConfig, agentFolder string, logger *slog.Logger) (*GenericProvider, error) {
	// Load .env from agent folder (optional)
	envPath := filepath.Join(agentFolder, genericEnvFile)
	_ = godotenv.Load(envPath)

	// API key is optional for custom endpoints (local servers may not need auth)
	apiKey := os.Getenv(genericAPIKey)

	// Build client options
	opts := []option.RequestOption{
		option.WithBaseURL(cfg.Agent.Provider), // Use provider string as base URL
	}
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}

	client := openai.NewClient(opts...)

	return &GenericProvider{
		client:   client,
		model:    cfg.Agent.Model,
		logger:   logger,
		endpoint: cfg.Agent.Provider,
	}, nil
}

// SendMessages sends messages to the generic endpoint and returns the response
func (p *GenericProvider) SendMessages(messages []Message, tools []config.ToolConfig) (string, []ToolCall, error) {
	chatMessages := make([]openai.ChatCompletionMessageParamUnion, len(messages))
	for i, msg := range messages {
		switch msg.Role {
		case RoleSystem:
			chatMessages[i] = openai.SystemMessage(msg.Content)
		case RoleUser:
			chatMessages[i] = openai.UserMessage(msg.Content)
		case RoleAssistant:
			chatMessages[i] = openai.AssistantMessage(msg.Content)
		default:
			return "", nil, fmt.Errorf("unknown role: %s", msg.Role)
		}
	}

	var chatTools []openai.ChatCompletionToolParam
	if len(tools) > 0 {
		chatTools = make([]openai.ChatCompletionToolParam, len(tools))
		for i, tool := range tools {
			chatTools[i] = convertToolToOpenAI(&tool)
		}
	}

	ctx := context.Background()
	params := openai.ChatCompletionNewParams{
		Model:    p.model,
		Messages: chatMessages,
	}
	if len(chatTools) > 0 {
		params.Tools = chatTools
	}

	// Log API call
	if p.logger != nil {
		p.logger.Debug("api call", "provider", "generic", "endpoint", p.endpoint, "model", p.model)
	}

	resp, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		if p.logger != nil {
			p.logger.Error("api error", "provider", "generic", "endpoint", p.endpoint, "error", err)
		}
		return "", nil, fmt.Errorf("generic provider api error: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", nil, fmt.Errorf("no response from api")
	}

	choice := resp.Choices[0]
	content := choice.Message.Content

	var toolCalls []ToolCall
	if len(choice.Message.ToolCalls) > 0 {
		toolCalls = make([]ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			var args map[string]interface{}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				return "", nil, fmt.Errorf("failed to parse tool arguments: %w", err)
			}
			toolCalls[i] = ToolCall{
				ID:   tc.ID,
				Name: tc.Function.Name,
				Args: args,
			}
		}
	}

	return content, toolCalls, nil
}
