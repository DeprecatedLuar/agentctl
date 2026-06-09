package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DeprecatedLuar/agentctl/internal/config"
	"github.com/joho/godotenv"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// OpenRouterProvider implements Provider for OpenRouter
type OpenRouterProvider struct {
	client openai.Client
	model  string
}

// NewOpenRouterProvider creates an OpenRouter provider
func NewOpenRouterProvider(cfg *config.AgentConfig, agentFolder string) (*OpenRouterProvider, error) {
	// Load .env from agent folder
	envPath := filepath.Join(agentFolder, ".env")
	_ = godotenv.Load(envPath)

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENROUTER_API_KEY not found in .env or environment")
	}

	client := openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL("https://openrouter.ai/api/v1"),
	)

	return &OpenRouterProvider{
		client: client,
		model:  cfg.Model,
	}, nil
}

// SendMessages sends messages to OpenRouter and returns the response
func (p *OpenRouterProvider) SendMessages(messages []Message, tools []config.ToolConfig) (string, []ToolCall, error) {
	chatMessages := make([]openai.ChatCompletionMessageParamUnion, len(messages))
	for i, msg := range messages {
		switch msg.Role {
		case "system":
			chatMessages[i] = openai.SystemMessage(msg.Content)
		case "user":
			chatMessages[i] = openai.UserMessage(msg.Content)
		case "assistant":
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

	resp, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", nil, fmt.Errorf("openrouter api error: %w", err)
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

func convertToolToOpenRouterFormat(tool *config.ToolConfig) openai.ChatCompletionToolParam {
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
		"type":       "object",
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
