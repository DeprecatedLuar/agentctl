package agent

import (
	"fmt"
	"strings"

	"github.com/DeprecatedLuar/agentctl/internal/config"
	"github.com/DeprecatedLuar/agentctl/internal/provider"
)

// Input represents user input to the agent
type Input struct {
	SessionKey string
	Content    string
}

// Message is an alias for provider.Message (for convenience)
type Message = provider.Message

// Run executes the agent with the given configuration and input
func Run(cfg *config.AgentConfig, tools []config.ToolConfig, prompt *config.ParsedPrompt, history []Message, input Input, agentFolder string) (string, error) {
	// Create provider
	prov, err := provider.NewProvider(cfg, agentFolder)
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
		inputContent := strings.ReplaceAll(prompt.Input.Content, "{{input}}", input.Content)
		messages = append(messages, Message{
			Role:    prompt.Input.Role,
			Content: inputContent,
		})
	} else {
		// Fallback if no input section defined
		messages = append(messages, Message{
			Role:    "user",
			Content: input.Content,
		})
	}

	// Agentic loop: send -> check for tool calls -> execute -> repeat
	maxIterations := 10 // prevent infinite loops
	for i := 0; i < maxIterations; i++ {
		response, toolCalls, err := prov.SendMessages(messages, tools)
		if err != nil {
			return "", fmt.Errorf("provider error: %w", err)
		}

		// If no tool calls, we're done
		if len(toolCalls) == 0 {
			return response, nil
		}

		// Add assistant message with tool calls to history
		messages = append(messages, Message{
			Role:    "assistant",
			Content: response,
		})

		// Execute all tool calls and collect results
		for _, tc := range toolCalls {
			tool := FindTool(tools, tc.Name)
			if tool == nil {
				return "", fmt.Errorf("unknown tool requested: %s", tc.Name)
			}

			result := ExecuteTool(tool, tc.Args)

			// Add tool result as a message
			// OpenAI expects tool results as role="tool" with tool_call_id
			// For simplicity, we'll format as a user message with context
			messages = append(messages, Message{
				Role:    "user",
				Content: fmt.Sprintf("Tool %s result:\n%s", tc.Name, result),
			})
		}

		// Loop continues - send messages with tool results
	}

	return "", fmt.Errorf("max iterations reached in agentic loop")
}
