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
	// OpenAI configuration
	openaiEnvFile   = ".env"
	openaiAPIKey    = "OPENAI_API_KEY"
	openaiBaseURL   = "https://api.openai.com/v1"
	openaiProvider  = "openai"

	// JSON schema types
	schemaTypeObject = "object"
)

// OpenAIProvider implements Provider for OpenAI
type OpenAIProvider struct {
	client openai.Client
	model  string
	logger *slog.Logger
}

// NewOpenAIProvider creates an OpenAI provider
func NewOpenAIProvider(cfg *config.AgentConfig, agentFolder string, logger *slog.Logger) (*OpenAIProvider, error) {
	// Load .env from agent folder
	envPath := filepath.Join(agentFolder, openaiEnvFile)
	_ = godotenv.Load(envPath)

	apiKey := os.Getenv(openaiAPIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("%s not found in %s or environment", openaiAPIKey, openaiEnvFile)
	}

	client := openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(openaiBaseURL),
	)

	return &OpenAIProvider{
		client: client,
		model:  cfg.Model,
		logger: logger,
	}, nil
}

// SendMessages sends messages to OpenAI and returns the response
func (p *OpenAIProvider) SendMessages(messages []Message, tools []config.ToolConfig) (string, []ToolCall, error) {
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
		p.logger.Debug("api call", "provider", openaiProvider, "model", p.model)
	}

	resp, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		if p.logger != nil {
			p.logger.Error("api error", "provider", openaiProvider, "error", err)
		}
		return "", nil, fmt.Errorf("%s api error: %w", openaiProvider, err)
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

func convertToolToOpenAI(tool *config.ToolConfig) openai.ChatCompletionToolParam {
	properties := make(map[string]interface{})
	required := []string{}

	for name, param := range tool.Parameters {
		properties[name] = map[string]interface{}{
			"type":        param.Type,
			"description": param.Description,
		}
		if param.Required {
			required = append(required, name)
		}
	}

	parametersSchema := map[string]interface{}{
		"type":       schemaTypeObject,
		"properties": properties,
	}
	if len(required) > 0 {
		parametersSchema["required"] = required
	}

	return openai.ChatCompletionToolParam{
		Function: openai.FunctionDefinitionParam{
			Name:        tool.Name,
			Description: openai.String(tool.Description),
			Parameters:  openai.FunctionParameters(parametersSchema),
		},
	}
}
