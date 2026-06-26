package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/DeprecatedLuar/agentctl/internal/config"
	"github.com/DeprecatedLuar/agentctl/internal/debug"
	"github.com/joho/godotenv"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const (
	// OpenRouter configuration
	openrouterEnvFile     = ".env"
	openrouterAPIKey      = "OPENROUTER_API_KEY"
	openrouterBaseURL     = "https://openrouter.ai/api/v1"
	openrouterProvider    = "openrouter"

	// JSON schema types (shared with OpenAI)
	schemaTypeObjectOR = "object"
)

// OpenRouterProvider implements Provider for OpenRouter
type OpenRouterProvider struct {
	client          openai.Client
	model           string
	logger          *slog.Logger
	agentFolder     string
	debugCallsEnabled bool
}

// NewOpenRouterProvider creates an OpenRouter provider
func NewOpenRouterProvider(cfg *config.AgentConfig, agentFolder string, logger *slog.Logger) (*OpenRouterProvider, error) {
	// Load .env from agent folder
	envPath := filepath.Join(agentFolder, openrouterEnvFile)
	_ = godotenv.Load(envPath)

	apiKey := os.Getenv(openrouterAPIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("%s not found in %s or environment", openrouterAPIKey, openrouterEnvFile)
	}

	client := openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(openrouterBaseURL),
	)

	return &OpenRouterProvider{
		client:            client,
		model:             cfg.Agent.Model,
		logger:            logger,
		agentFolder:       agentFolder,
		debugCallsEnabled: cfg.Agent.IsDebugMode(),
	}, nil
}

// SendMessages sends messages to OpenRouter and returns the response
func (p *OpenRouterProvider) SendMessages(messages []Message, tools []config.ToolConfig) (string, []ToolCall, error) {
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
			chatTools[i] = convertToolToOpenRouterFormat(&tool)
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

	// Log API call with debug details
	if p.logger != nil {
		p.logger.Debug("api call", "provider", openrouterProvider, "model", p.model)

		// Convert messages for debug logging
		debugMessages := make([]debug.Message, len(messages))
		for i, msg := range messages {
			debugMessages[i] = debug.Message{
				Role:    string(msg.Role),
				Content: msg.Content,
			}
		}
		debug.LogRequest(p.logger, debugMessages, tools, openrouterProvider, p.model, "", p.agentFolder, p.debugCallsEnabled)
	}

	resp, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		if p.logger != nil {
			p.logger.Error("api error", "provider", openrouterProvider, "error", err)
			debug.LogResponse(p.logger, "", nil, err, p.agentFolder, p.debugCallsEnabled)
		}
		return "", nil, fmt.Errorf("%s api error: %w", openrouterProvider, err)
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

	// Log response with debug details
	if p.logger != nil {
		debugToolCalls := make([]debug.ToolCall, len(toolCalls))
		for i, tc := range toolCalls {
			debugToolCalls[i] = debug.ToolCall{
				Name: tc.Name,
				Args: tc.Args,
			}
		}
		debug.LogResponse(p.logger, content, debugToolCalls, nil, p.agentFolder, p.debugCallsEnabled)
	}

	return content, toolCalls, nil
}

func convertToolToOpenRouterFormat(tool *config.ToolConfig) openai.ChatCompletionToolParam {
	// Use shared converter (filters disabled and return-override parameters)
	properties, required := config.ConvertToolParameters(tool)

	parametersSchema := map[string]interface{}{
		"type":       schemaTypeObjectOR,
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
