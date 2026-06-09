package interfaces

import (
	"context"
	"database/sql"

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
	DB          *sql.DB
}

// Run executes the agent with memory management
func (r *Runner) Run(input agent.Input) (string, error) {
	// Load history from memory
	var history []agent.Message
	if r.Config.Memory.MaxMessages > 0 && r.DB != nil {
		messages, err := memory.Load(r.DB, input.SessionKey, r.Config.Memory.MaxMessages)
		if err != nil {
			return "", err
		}
		history = messages
	}

	// Call agent
	response, err := agent.Run(r.Config, r.Tools, r.Prompt, history, input, r.AgentFolder)
	if err != nil {
		return "", err
	}

	// Save to memory
	if r.Config.Memory.MaxMessages > 0 && r.DB != nil {
		_ = memory.Save(r.DB, input.SessionKey, "user", input.Content)
		_ = memory.Save(r.DB, input.SessionKey, "assistant", response)
	}

	return response, nil
}
