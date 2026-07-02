package debug

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DeprecatedLuar/agentctl/internal/config"
)

const (
	lastCallFile = "last-call.json"
)

// RequestData contains the full request payload for debugging
type RequestData struct {
	Messages []Message        `json:"messages"`
	Tools    []ToolDefinition `json:"tools"`
	Provider string           `json:"provider"`
	Model    string           `json:"model"`
}

// ResponseData contains the full response payload for debugging
type ResponseData struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Error     string     `json:"error,omitempty"`
}

// ExchangeRecord merges one request/response pair into a single record
type ExchangeRecord struct {
	Timestamp string       `json:"timestamp"`
	Request   RequestData  `json:"request"`
	Response  ResponseData `json:"response"`
}

// Message represents a chat message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ToolDefinition represents a tool with its parameters
type ToolDefinition struct {
	Name        string                        `json:"name"`
	Description string                        `json:"description"`
	Parameters  map[string]ParameterDefinition `json:"parameters"`
}

// ParameterDefinition represents a tool parameter
type ParameterDefinition struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// ToolCall represents a tool invocation
type ToolCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

// RecordExchange writes the merged request/response for one API call to the
// user's last-call.json, overwriting any previous exchange. It also emits a
// concise slog.Debug summary regardless of whether the file write is enabled.
func RecordExchange(logger *slog.Logger, userFolder string, messages []Message, tools []config.ToolConfig, provider, model string, resp ResponseData, callErr error, enabled bool) {
	if logger != nil {
		msgPreview := buildMessagePreview(messages)
		toolNames := buildToolNames(tools)
		logger.Debug("request details",
			"messages_count", len(messages),
			"messages_preview", msgPreview,
			"tools", toolNames,
			"provider", provider,
			"model", model,
		)

		if callErr != nil {
			logger.Debug("response details", "error", callErr.Error())
		} else if len(resp.ToolCalls) > 0 {
			toolCallNames := make([]string, len(resp.ToolCalls))
			for i, tc := range resp.ToolCalls {
				toolCallNames[i] = tc.Name
			}
			logger.Debug("response details",
				"content_length", len(resp.Content),
				"tool_calls", toolCallNames,
			)
		} else {
			logger.Debug("response details",
				"content_length", len(resp.Content),
				"content_preview", truncate(resp.Content, 100),
			)
		}
	}

	if !enabled {
		return
	}

	record := ExchangeRecord{
		Timestamp: time.Now().Format(time.RFC3339),
		Request: RequestData{
			Messages: messages,
			Tools:    convertTools(tools),
			Provider: provider,
			Model:    model,
		},
		Response: resp,
	}
	if callErr != nil {
		record.Response.Error = callErr.Error()
	}

	_ = writeLastCall(userFolder, record)
}

// LogToolExecution logs tool execution details
func LogToolExecution(logger *slog.Logger, toolName, command, result string, exitCode int) {
	if logger == nil {
		return
	}

	logger.Debug("tool execution",
		"tool", toolName,
		"command", command,
		"exit_code", exitCode,
		"result_length", len(result),
		"result_preview", truncate(result, 200),
	)
}

// writeLastCall writes the exchange record to <userFolder>/last-call.json, overwriting any previous call.
func writeLastCall(userFolder string, record ExchangeRecord) error {
	if err := os.MkdirAll(userFolder, 0755); err != nil {
		return fmt.Errorf("create user folder: %w", err)
	}

	jsonData, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal exchange record: %w", err)
	}

	filePath := filepath.Join(userFolder, lastCallFile)
	if err := os.WriteFile(filePath, jsonData, 0644); err != nil {
		return fmt.Errorf("write last-call file: %w", err)
	}

	return nil
}

// buildMessagePreview creates a preview of messages for logging
func buildMessagePreview(messages []Message) string {
	if len(messages) == 0 {
		return "[]"
	}

	first := messages[0]
	last := messages[len(messages)-1]

	if len(messages) == 1 {
		return fmt.Sprintf("[%s: %s]", first.Role, truncate(first.Content, 50))
	}

	return fmt.Sprintf("[%s: %s ... %s: %s]",
		first.Role, truncate(first.Content, 30),
		last.Role, truncate(last.Content, 30),
	)
}

// buildToolNames creates a comma-separated list of tool names
func buildToolNames(tools []config.ToolConfig) string {
	if len(tools) == 0 {
		return "[]"
	}

	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
	}
	return "[" + strings.Join(names, ", ") + "]"
}

// convertTools converts config.ToolConfig to ToolDefinition for JSON output.
// Applies same filtering as provider converters (excludes disabled and return-override params).
func convertTools(tools []config.ToolConfig) []ToolDefinition {
	result := make([]ToolDefinition, len(tools))
	for i, tool := range tools {
		params := make(map[string]ParameterDefinition)
		for name, param := range tool.Parameters {
			// Skip disabled parameters OR parameters with return override (match AI filtering)
			if !param.Enabled || param.Return != "" {
				continue
			}
			params[name] = ParameterDefinition{
				Type:        param.Type,
				Description: param.Description,
				Required:    param.Required,
			}
		}
		result[i] = ToolDefinition{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  params,
		}
	}
	return result
}

// truncate shortens a string to maxLen, adding "..." if truncated
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
