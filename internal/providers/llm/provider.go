package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/DeprecatedLuar/agentctl/internal/config"
	"github.com/DeprecatedLuar/agentctl/internal/debug"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const (
	// Provider names
	ProviderOpenAI     = "openai"
	ProviderOpenRouter = "openrouter"

	// Message roles
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"

	// JSON schema types
	schemaTypeObject = "object"
)

// Message represents a single message in the conversation
type Message struct {
	Role    string
	Content string
}

// ToolCall represents a tool invocation from the AI
type ToolCall struct {
	ID   string
	Name string
	Args map[string]interface{}
}

// Provider handles communication with AI providers
type Provider interface {
	// SendMessages sends messages and returns the assistant's response
	// If tool calls are requested, returns them alongside the response
	SendMessages(messages []Message, tools []config.ToolConfig) (response string, toolCalls []ToolCall, err error)
}

// baseProvider holds the state and call logic shared by all Provider implementations.
// Each concrete provider only differs in how its openai.Client is constructed.
type baseProvider struct {
	client       openai.Client
	model        string
	providerName string
	logger       *slog.Logger
	userFolder   string
	debugEnabled bool
}

// buildClient constructs a shared openai.Client with the given auth/base-URL options.
func buildClient(apiKey, baseURL string) openai.Client {
	opts := []option.RequestOption{option.WithBaseURL(baseURL)}
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	return openai.NewClient(opts...)
}

// SendMessages implements Provider using the shared call sequence. All three
// concrete providers (OpenAI, OpenRouter, Generic) delegate here.
func (p *baseProvider) SendMessages(messages []Message, tools []config.ToolConfig) (string, []ToolCall, error) {
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
			chatTools[i] = convertTool(&tool)
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

	if p.logger != nil {
		p.logger.Debug("api call", "provider", p.providerName, "model", p.model)
	}

	debugMessages := make([]debug.Message, len(messages))
	for i, msg := range messages {
		debugMessages[i] = debug.Message{Role: msg.Role, Content: msg.Content}
	}

	resp, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		if p.logger != nil {
			p.logger.Error("api error", "provider", p.providerName, "error", err)
		}
		debug.RecordExchange(p.logger, p.userFolder, debugMessages, tools, p.providerName, p.model, debug.ResponseData{}, err, p.debugEnabled)
		return "", nil, fmt.Errorf("%s api error: %w", p.providerName, err)
	}

	if len(resp.Choices) == 0 {
		noChoicesErr := fmt.Errorf("no response from api")
		debug.RecordExchange(p.logger, p.userFolder, debugMessages, tools, p.providerName, p.model, debug.ResponseData{}, noChoicesErr, p.debugEnabled)
		return "", nil, noChoicesErr
	}

	choice := resp.Choices[0]
	content := choice.Message.Content
	reasoning := extractReasoning(choice.Message)

	var toolCalls []ToolCall
	if len(choice.Message.ToolCalls) > 0 {
		toolCalls = make([]ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			var args map[string]interface{}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				parseErr := fmt.Errorf("failed to parse tool arguments: %w", err)
				debug.RecordExchange(p.logger, p.userFolder, debugMessages, tools, p.providerName, p.model, debug.ResponseData{}, parseErr, p.debugEnabled)
				return "", nil, parseErr
			}
			toolCalls[i] = ToolCall{
				ID:   tc.ID,
				Name: tc.Function.Name,
				Args: args,
			}
		}
	}

	debugToolCalls := make([]debug.ToolCall, len(toolCalls))
	for i, tc := range toolCalls {
		debugToolCalls[i] = debug.ToolCall{Name: tc.Name, Args: tc.Args}
	}
	debug.RecordExchange(p.logger, p.userFolder, debugMessages, tools, p.providerName, p.model,
		debug.ResponseData{Content: content, Reasoning: reasoning, ToolCalls: debugToolCalls}, nil, p.debugEnabled)

	return content, toolCalls, nil
}

// extractReasoning pulls the provider-extension "reasoning" field (e.g. OpenRouter's
// reasoning-model output) out of a chat completion message. The openai-go SDK has no
// native field for it since it's not part of the standard OpenAI response schema, so
// it surfaces only in JSON.ExtraFields. Returns "" if the provider didn't send one.
func extractReasoning(msg openai.ChatCompletionMessage) string {
	field, ok := msg.JSON.ExtraFields["reasoning"]
	if !ok {
		return ""
	}
	var reasoning string
	if err := json.Unmarshal([]byte(field.Raw()), &reasoning); err != nil {
		return ""
	}
	return reasoning
}

// convertTool converts a tool config to OpenAI-compatible schema format.
func convertTool(tool *config.ToolConfig) openai.ChatCompletionToolParam {
	properties, required := config.ConvertToolParameters(tool)

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

// NewProvider creates a provider based on the config. recordExchanges controls
// whether this provider's calls are written to the user's last-call.json —
// pass false for incidental calls (e.g. session title generation) that would
// otherwise clobber the debug record of the actual conversation.
func NewProvider(cfg *config.AgentConfig, agentFolder, userFolder string, recordExchanges bool, logger *slog.Logger) (Provider, error) {
	// Check if provider is a custom endpoint (http/https URL)
	if strings.HasPrefix(cfg.Agent.Provider, "http://") || strings.HasPrefix(cfg.Agent.Provider, "https://") {
		return NewGenericProvider(cfg, agentFolder, userFolder, recordExchanges, logger)
	}

	// Named providers
	switch cfg.Agent.Provider {
	case ProviderOpenAI:
		return NewOpenAIProvider(cfg, agentFolder, userFolder, recordExchanges, logger)
	case ProviderOpenRouter:
		return NewOpenRouterProvider(cfg, agentFolder, userFolder, recordExchanges, logger)
	default:
		return nil, fmt.Errorf("unsupported provider: %s (must be 'openai', 'openrouter', or an http/https URL)", cfg.Agent.Provider)
	}
}
