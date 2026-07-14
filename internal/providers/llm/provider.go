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

	// ReasoningDetails is the raw "reasoning_details" JSON array a provider
	// returned for this assistant turn. Stored verbatim and echoed back
	// unmodified on the next request — never synthesized or parsed. Only
	// ever set on Role == RoleAssistant messages returned by a provider that
	// supports reasoning preservation (see baseProvider.supportsReasoning).
	ReasoningDetails json.RawMessage
}

// ToolCall represents a tool invocation from the AI
type ToolCall struct {
	ID   string
	Name string
	Args map[string]interface{}
}

// Provider handles communication with AI providers
type Provider interface {
	// SendMessages sends messages and returns the assistant's response.
	// If tool calls are requested, returns them alongside the response.
	// reasoningDetails is the raw "reasoning_details" payload for this turn
	// (nil if the provider/model didn't return one) — callers should store it
	// verbatim on the resulting assistant Message so it can be echoed back on
	// the next call, never parsed or modified.
	SendMessages(messages []Message, tools []config.ToolConfig) (response string, toolCalls []ToolCall, reasoningDetails json.RawMessage, err error)
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

	// Reasoning preservation ([advanced] reasoning/reasoning_effort). Only
	// OpenRouter's Chat Completions extension supports round-tripping
	// reasoning_details today (see NewOpenRouterProvider) — every other
	// provider leaves supportsReasoning false, making this a no-op.
	supportsReasoning bool
	reasoningEnabled  bool
	reasoningEffort   string
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
func (p *baseProvider) SendMessages(messages []Message, tools []config.ToolConfig) (string, []ToolCall, json.RawMessage, error) {
	chatMessages := make([]openai.ChatCompletionMessageParamUnion, len(messages))
	for i, msg := range messages {
		switch msg.Role {
		case RoleSystem:
			chatMessages[i] = openai.SystemMessage(msg.Content)
		case RoleUser:
			chatMessages[i] = openai.UserMessage(msg.Content)
		case RoleAssistant:
			chatMessages[i] = p.assistantMessage(msg)
		default:
			return "", nil, nil, fmt.Errorf("unknown role: %s", msg.Role)
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
	if p.supportsReasoning && p.reasoningEnabled {
		reasoningParam := map[string]any{}
		if p.reasoningEffort != "" {
			reasoningParam["effort"] = p.reasoningEffort
		} else {
			reasoningParam["enabled"] = true
		}
		params.SetExtraFields(map[string]any{"reasoning": reasoningParam})
	}

	if p.logger != nil {
		p.logger.Debug("api call", "provider", p.providerName, "model", p.model)
	}

	debugMessages := make([]debug.Message, len(messages))
	for i, msg := range messages {
		debugMessages[i] = debug.Message{
			Role:         msg.Role,
			Content:      msg.Content,
			HadReasoning: len(msg.ReasoningDetails) > 0,
		}
	}

	resp, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		if p.logger != nil {
			p.logger.Error("api error", "provider", p.providerName, "error", err)
		}
		debug.RecordExchange(p.logger, p.userFolder, debugMessages, tools, p.providerName, p.model, debug.ResponseData{}, err, p.debugEnabled)
		return "", nil, nil, fmt.Errorf("%s api error: %w", p.providerName, err)
	}

	if len(resp.Choices) == 0 {
		noChoicesErr := fmt.Errorf("no response from api")
		debug.RecordExchange(p.logger, p.userFolder, debugMessages, tools, p.providerName, p.model, debug.ResponseData{}, noChoicesErr, p.debugEnabled)
		return "", nil, nil, noChoicesErr
	}

	choice := resp.Choices[0]
	content := choice.Message.Content
	reasoning := extractReasoning(choice.Message)
	reasoningDetails := extractReasoningDetails(choice.Message)

	if p.logger != nil {
		if n := reasoningBlockCount(reasoningDetails); n > 0 {
			p.logger.Debug("reasoning preserved", "blocks", n)
		}
	}

	var toolCalls []ToolCall
	if len(choice.Message.ToolCalls) > 0 {
		toolCalls = make([]ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			var args map[string]interface{}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				parseErr := fmt.Errorf("failed to parse tool arguments: %w", err)
				debug.RecordExchange(p.logger, p.userFolder, debugMessages, tools, p.providerName, p.model, debug.ResponseData{}, parseErr, p.debugEnabled)
				return "", nil, nil, parseErr
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

	return content, toolCalls, reasoningDetails, nil
}

// assistantMessage converts an outbound assistant Message into an OpenAI chat
// param, attaching reasoning_details verbatim via SetExtraFields when present
// (see Message.ReasoningDetails). The typed openai.AssistantMessage helper has
// no field for reasoning_details since it's a provider extension, not part of
// the standard schema.
func (p *baseProvider) assistantMessage(msg Message) openai.ChatCompletionMessageParamUnion {
	param := openai.AssistantMessage(msg.Content)
	if p.supportsReasoning && len(msg.ReasoningDetails) > 0 {
		if signed := stripUnsignedReasoning(msg.ReasoningDetails); len(signed) > 0 {
			param.OfAssistant.SetExtraFields(map[string]any{"reasoning_details": json.RawMessage(signed)})
		}
	}
	return param
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

// reasoningBlockCount reports how many entries a reasoning_details payload
// carries, for the "reasoning preserved: N blocks" debug log. Returns 0 for
// nil/malformed input rather than erroring — this is observability only.
func reasoningBlockCount(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return 0
	}
	return len(entries)
}

// extractReasoningDetails pulls the raw "reasoning_details" array out of a chat
// completion message's extension fields (OpenRouter's structured reasoning
// payload). Returned verbatim, unparsed — callers must never modify it, only
// store and echo it back (see Message.ReasoningDetails). Returns nil if the
// provider didn't send one.
func extractReasoningDetails(msg openai.ChatCompletionMessage) json.RawMessage {
	field, ok := msg.JSON.ExtraFields["reasoning_details"]
	if !ok {
		return nil
	}
	return json.RawMessage(field.Raw())
}

// reasoningDetail mirrors just the fields of an OpenRouter reasoning_details
// entry needed to validate it's safe to resend (see stripUnsignedReasoning).
type reasoningDetail struct {
	Type      string `json:"type"`
	Signature string `json:"signature"`
}

// stripUnsignedReasoning drops any "reasoning.text" entries missing their
// signature before a reasoning_details payload is echoed back to the
// provider. An unsigned text block will be rejected by signature-validating
// providers (e.g. Anthropic-backed models via OpenRouter) with an "invalid
// signature" error; other entry types (encrypted, summary) pass through
// unchanged since they carry no signature to validate.
func stripUnsignedReasoning(raw json.RawMessage) json.RawMessage {
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil
	}

	kept := make([]json.RawMessage, 0, len(entries))
	for _, entry := range entries {
		var d reasoningDetail
		if err := json.Unmarshal(entry, &d); err != nil {
			continue // malformed entry, drop rather than risk a rejected request
		}
		if d.Type == "reasoning.text" && d.Signature == "" {
			continue
		}
		kept = append(kept, entry)
	}

	if len(kept) == 0 {
		return nil
	}
	out, err := json.Marshal(kept)
	if err != nil {
		return nil
	}
	return out
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
