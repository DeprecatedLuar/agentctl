package debug

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DeprecatedLuar/agentctl/internal/config"
)

const (
	debugDir      = ".data/debug-calls"
	maxDebugFiles = 10 // Keep last N debug files
)

// RequestData contains the full request payload for debugging
type RequestData struct {
	Timestamp string                 `json:"timestamp"`
	Messages  []Message              `json:"messages"`
	Tools     []ToolDefinition       `json:"tools"`
	Provider  string                 `json:"provider"`
	Model     string                 `json:"model"`
	Session   string                 `json:"session,omitempty"`
}

// ResponseData contains the full response payload for debugging
type ResponseData struct {
	Timestamp string     `json:"timestamp"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Error     string     `json:"error,omitempty"`
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

// LogRequest logs the request details at debug level and optionally writes to file
func LogRequest(logger *slog.Logger, messages []Message, tools []config.ToolConfig, provider, model, session string, agentFolder string, writeFile bool) {
	if logger == nil {
		return
	}

	// Log summary with message preview
	msgPreview := buildMessagePreview(messages)
	toolNames := buildToolNames(tools)

	logger.Debug("request details",
		"messages_count", len(messages),
		"messages_preview", msgPreview,
		"tools", toolNames,
		"provider", provider,
		"model", model,
	)

	// Optionally write full request to JSON file
	if writeFile {
		data := RequestData{
			Timestamp: time.Now().Format(time.RFC3339),
			Messages:  messages,
			Tools:     convertTools(tools),
			Provider:  provider,
			Model:     model,
			Session:   session,
		}
		_ = writeDebugFile(agentFolder, "request", data)
	}
}

// LogResponse logs the response details at debug level and optionally writes to file
func LogResponse(logger *slog.Logger, content string, toolCalls []ToolCall, err error, agentFolder string, writeFile bool) {
	if logger == nil {
		return
	}

	if err != nil {
		logger.Debug("response details", "error", err.Error())
	} else if len(toolCalls) > 0 {
		toolNames := make([]string, len(toolCalls))
		for i, tc := range toolCalls {
			toolNames[i] = tc.Name
		}
		logger.Debug("response details",
			"content_length", len(content),
			"tool_calls", toolNames,
		)
	} else {
		logger.Debug("response details",
			"content_length", len(content),
			"content_preview", truncate(content, 100),
		)
	}

	// Optionally write full response to JSON file
	if writeFile {
		data := ResponseData{
			Timestamp: time.Now().Format(time.RFC3339),
			Content:   content,
			ToolCalls: toolCalls,
		}
		if err != nil {
			data.Error = err.Error()
		}
		_ = writeDebugFile(agentFolder, "response", data)
	}
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

// CleanupDebugFiles removes old debug files, keeping only the last N
func CleanupDebugFiles(agentFolder string) error {
	debugPath := filepath.Join(agentFolder, debugDir)

	// Check if directory exists
	if _, err := os.Stat(debugPath); os.IsNotExist(err) {
		return nil // Nothing to clean up
	}

	entries, err := os.ReadDir(debugPath)
	if err != nil {
		return fmt.Errorf("read debug directory: %w", err)
	}

	// Filter JSON files and sort by name (timestamp-based)
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)

	// Remove old files if we have more than maxDebugFiles
	if len(files) > maxDebugFiles {
		for i := 0; i < len(files)-maxDebugFiles; i++ {
			filePath := filepath.Join(debugPath, files[i])
			_ = os.Remove(filePath)
		}
	}

	return nil
}

// writeDebugFile writes debug data to a timestamped JSON file
func writeDebugFile(agentFolder, fileType string, data interface{}) error {
	debugPath := filepath.Join(agentFolder, debugDir)

	// Ensure directory exists
	if err := os.MkdirAll(debugPath, 0755); err != nil {
		return fmt.Errorf("create debug directory: %w", err)
	}

	// Create timestamped filename
	timestamp := time.Now().Format("2006-01-02T15-04-05")
	filename := fmt.Sprintf("%s-%s.json", timestamp, fileType)
	filePath := filepath.Join(debugPath, filename)

	// Marshal to JSON with pretty printing
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal debug data: %w", err)
	}

	// Write to file
	if err := os.WriteFile(filePath, jsonData, 0644); err != nil {
		return fmt.Errorf("write debug file: %w", err)
	}

	return nil
}

// buildMessagePreview creates a preview of messages for logging
func buildMessagePreview(messages []Message) string {
	if len(messages) == 0 {
		return "[]"
	}

	// Show first and last message roles/content preview
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

// convertTools converts config.ToolConfig to ToolDefinition for JSON output
func convertTools(tools []config.ToolConfig) []ToolDefinition {
	result := make([]ToolDefinition, len(tools))
	for i, tool := range tools {
		params := make(map[string]ParameterDefinition)
		for name, param := range tool.Parameters {
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
