package agent

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/DeprecatedLuar/agentctl/internal/config"
	"github.com/DeprecatedLuar/agentctl/internal/providers/llm"
)

const (
	// Agentic loop configuration
	maxIterations = 10 // Prevent infinite loops

	// Message roles
	roleUser      = "user"
	roleAssistant = "assistant"

	// Input placeholder
	inputPlaceholder = "{{$input}}"

	// Format strings
	toolResultFormat = "Tool %s result:\n%s"
)

// Input represents user input to the agent
type Input struct {
	UserID    string
	SessionID string
	Interface string
	Content   string
}

// Message is an alias for llm.Message (for convenience)
type Message = llm.Message

// Run executes the agent with the given configuration and input
func Run(cfg *config.AgentConfig, tools []config.ToolConfig, prompt *config.ParsedPrompt, history []Message, input Input, agentFolder string, logger *slog.Logger, verbose bool, debug bool) (string, error) {
	// Create provider
	prov, err := llm.NewProvider(cfg, agentFolder, logger)
	if err != nil {
		return "", fmt.Errorf("failed to create provider: %w", err)
	}

	// Build messages array
	messages := []Message{}

	// 1. Static messages from prompt
	for _, section := range prompt.Static {
		messages = append(messages, Message{
			Role:    section.Role,
			Content: section.Content,
		})
	}

	// 2. History messages
	messages = append(messages, history...)

	// 3. Input message with {{input}} substitution
	if prompt.Input != nil {
		inputContent := strings.ReplaceAll(prompt.Input.Content, inputPlaceholder, input.Content)
		messages = append(messages, Message{
			Role:    prompt.Input.Role,
			Content: inputContent,
		})
	} else {
		// Fallback if no input section defined
		messages = append(messages, Message{
			Role:    roleUser,
			Content: input.Content,
		})
	}

	// Agentic loop: send -> check for tool calls -> execute -> repeat
	for i := 0; i < maxIterations; i++ {
		// Log provider request
		if logger != nil {
			logger.Info("provider request", "messages", len(messages), "tools", len(tools))
		}

		response, toolCalls, err := prov.SendMessages(messages, tools)
		if err != nil {
			if logger != nil {
				logger.Error("provider error", "error", err)
			}
			return "", fmt.Errorf("provider error: %w", err)
		}

		// Log provider response
		if logger != nil {
			if len(toolCalls) > 0 {
				toolNames := make([]string, len(toolCalls))
				for i, tc := range toolCalls {
					toolNames[i] = tc.Name
				}
				logger.Info("provider response", "tools", toolNames)
			} else {
				logger.Info("provider response")
			}
		}

		// If no tool calls, we're done
		if len(toolCalls) == 0 {
			if logger != nil {
				logger.Info("response sent")
			}
			return response, nil
		}

		// Add assistant message with tool calls to history
		messages = append(messages, Message{
			Role:    roleAssistant,
			Content: response,
		})

		// Execute all tool calls and collect results
		for _, tc := range toolCalls {
			tool := FindTool(tools, tc.Name)
			if tool == nil {
				return "", fmt.Errorf("unknown tool requested: %s", tc.Name)
			}

			result := ExecuteTool(tool, tc.Args, agentFolder, logger, verbose, debug)

			// Add tool result as a message
			// OpenAI expects tool results as role="tool" with tool_call_id
			// For simplicity, we'll format as a user message with context
			messages = append(messages, Message{
				Role:    roleUser,
				Content: fmt.Sprintf(toolResultFormat, tc.Name, result),
			})
		}

		// Loop continues - send messages with tool results
	}

	return "", fmt.Errorf("max iterations reached in agentic loop")
}
