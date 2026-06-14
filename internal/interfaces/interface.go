package interfaces

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/DeprecatedLuar/agentctl/internal/agent"
	"github.com/DeprecatedLuar/agentctl/internal/config"
	"github.com/DeprecatedLuar/agentctl/internal/session"
)

// Interface defines the contract for agent interfaces (CLI, Telegram, etc.)
type Interface interface {
	Start(ctx context.Context, runner *Runner) error
}

// Runner handles agent execution with memory persistence
type Runner struct {
	AgentFolder string
	Logger      *slog.Logger
	Verbose     bool
	Debug       bool
}

// Run executes the agent with memory management
func (r *Runner) Run(input agent.Input) (string, error) {
	// Load config, tools, and prompt fresh on each request
	agentCfg, agentIssues := config.LoadAgent(r.AgentFolder)
	if agentCfg == nil {
		err := formatValidationError("agent configuration", agentIssues)
		r.Logger.Error("failed to load agent config", "error", err)
		return "", err
	}

	tools, toolIssues := config.LoadTools(r.AgentFolder, agentCfg.Tools)
	prompt, promptIssues := config.Parse(r.AgentFolder, map[string]string{})

	// Collect all validation issues
	allIssues := append(agentIssues, toolIssues...)
	allIssues = append(allIssues, promptIssues...)

	// Check for errors
	for _, issue := range allIssues {
		if issue.Type == config.IssueError {
			err := formatValidationError("configuration", allIssues)
			r.Logger.Error("configuration validation failed", "error", err)
			return "", err
		}
	}

	// Load history from session
	var history []agent.Message
	if agentCfg.Memory.MaxMessages > 0 {
		messages, err := session.Load(r.AgentFolder, input.UserID, input.SessionID, agentCfg.Memory.MaxMessages)
		if err != nil {
			r.Logger.Error("failed to load session history", "error", err)
			return "", err
		}

		// Convert session.Message to agent.Message
		history = make([]agent.Message, len(messages))
		for i, msg := range messages {
			history[i] = agent.Message{
				Role:    msg.Role,
				Content: msg.Content,
			}
		}
	}

	// Call agent
	response, err := agent.Run(agentCfg, tools, prompt, history, input, r.AgentFolder, r.Logger, r.Verbose, r.Debug)
	if err != nil {
		r.Logger.Error("agent execution failed", "error", err)
		return "", err
	}

	// Save to session
	if agentCfg.Memory.MaxMessages > 0 {
		_ = session.Save(r.AgentFolder, input.UserID, input.SessionID, "user", input.Content)
		_ = session.Save(r.AgentFolder, input.UserID, input.SessionID, "assistant", response)
	}

	return response, nil
}

// formatValidationError creates an error message from validation issues
func formatValidationError(context string, issues []config.ValidationIssue) error {
	var msgs []string
	for _, issue := range issues {
		if issue.Type == config.IssueError {
			msgs = append(msgs, issue.Message)
		}
	}
	if len(msgs) == 0 {
		return fmt.Errorf("%s failed", context)
	}
	return fmt.Errorf("%s failed: %s", context, strings.Join(msgs, "; "))
}
