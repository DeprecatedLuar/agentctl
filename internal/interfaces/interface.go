package interfaces

import (
	"context"
	"log/slog"

	"github.com/DeprecatedLuar/agentctl/internal/agent"
	"github.com/DeprecatedLuar/agentctl/internal/config"
	"github.com/DeprecatedLuar/agentctl/internal/memory"
)

// Interface defines the contract for agent interfaces (CLI, Telegram, etc.)
type Interface interface {
	Start(ctx context.Context, runner *Runner) error
}

// Runner handles agent execution with memory persistence
type Runner struct {
	AgentFolder string
	Config      *config.AgentConfig
	Tools       []config.ToolConfig
	Prompt      *config.ParsedPrompt
	Logger      *slog.Logger
	Verbose     bool
	Debug       bool
}

// Run executes the agent with memory management
func (r *Runner) Run(input agent.Input) (string, error) {
	// Load history from memory
	var history []agent.Message
	if r.Config.Memory.MaxMessages > 0 {
		messages, err := memory.Load(r.AgentFolder, input.SessionKey, r.Config.Memory.MaxMessages)
		if err != nil {
			return "", err
		}
		history = messages
	}

	// Call agent
	response, err := agent.Run(r.Config, r.Tools, r.Prompt, history, input, r.AgentFolder, r.Logger, r.Verbose, r.Debug)
	if err != nil {
		return "", err
	}

	// Save to memory
	if r.Config.Memory.MaxMessages > 0 {
		_ = memory.Save(r.AgentFolder, input.SessionKey, "user", input.Content)
		_ = memory.Save(r.AgentFolder, input.SessionKey, "assistant", response)
	}

	return response, nil
}
