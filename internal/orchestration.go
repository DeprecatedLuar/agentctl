package internal

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/DeprecatedLuar/agentctl/internal/agent"
	"github.com/DeprecatedLuar/agentctl/internal/config"
	"github.com/DeprecatedLuar/agentctl/internal/session"
)

// Orchestrator orchestrates a single conversation turn
type Orchestrator struct {
	AgentFolder  string
	SessionStore session.SessionStore
	Logger       *slog.Logger
	Verbose      bool
	Debug        bool
}

// HandleMessage implements the MessageHandler interface
// Executes a conversation turn with full orchestration:
// - Resolves session from contact ID
// - Loads config/tools/prompt (hot reload)
// - Loads session history
// - Calls agent
// - Saves messages
// - Triggers title generation (async, first turn only)
func (o *Orchestrator) HandleMessage(iface, contactID, displayName, content string) (string, error) {
	// Resolve session from contact ID
	resolved, err := session.Resolve(o.SessionStore, o.AgentFolder, iface, contactID, displayName)
	if err != nil {
		o.Logger.Error("session resolution failed", "contact", contactID, "error", err)
		return "", fmt.Errorf("session resolution failed: %w", err)
	}

	return o.handleMessageInternal(resolved.UserID, resolved.SessionID, iface, content)
}

// HandleExplicitMessage implements the MessageHandler interface for explicit resolution
// Used by CLI interface when --user/--session flags are provided
// Bypasses contact resolution and uses provided IDs directly
func (o *Orchestrator) HandleExplicitMessage(userID, sessionID, iface, content string) (string, error) {
	return o.handleMessageInternal(userID, sessionID, iface, content)
}

// handleMessageInternal contains the core message handling logic
// Called by both HandleMessage (after resolution) and HandleExplicitMessage (direct)
func (o *Orchestrator) handleMessageInternal(userID, sessionID, iface, content string) (string, error) {

	// Load config, tools, and prompt fresh on each request (hot reload)
	agentCfg, agentIssues := config.LoadAgent(o.AgentFolder)
	if agentCfg == nil {
		err := formatValidationError("agent configuration", agentIssues)
		o.Logger.Error("failed to load agent config", "error", err)
		return "", err
	}

	tools, toolIssues := config.LoadTools(o.AgentFolder, agentCfg.Tools)
	prompt, promptIssues := config.Parse(o.AgentFolder, map[string]string{})

	// Collect all validation issues
	allIssues := append(agentIssues, toolIssues...)
	allIssues = append(allIssues, promptIssues...)

	// Check for errors
	for _, issue := range allIssues {
		if issue.Type == config.IssueError {
			err := formatValidationError("configuration", allIssues)
			o.Logger.Error("configuration validation failed", "error", err)
			return "", err
		}
	}

	// Load history from session
	var history []agent.Message
	if agentCfg.Memory.MaxMessages > 0 {
		messages, err := o.SessionStore.Load(userID, sessionID, agentCfg.Memory.MaxMessages)
		if err != nil {
			o.Logger.Error("failed to load session history", "error", err)
			return "", err
		}

		// Convert internal.Message to agent.Message
		history = make([]agent.Message, len(messages))
		for i, msg := range messages {
			history[i] = agent.Message{
				Role:    msg.Role,
				Content: msg.Content,
			}
		}
	}

	// Build agent input
	input := agent.Input{
		UserID:    userID,
		SessionID: sessionID,
		Interface: iface,
		Content:   content,
	}

	// Call agent
	response, err := agent.Run(agentCfg, tools, prompt, history, input, o.AgentFolder, o.Logger, o.Verbose, o.Debug)
	if err != nil {
		o.Logger.Error("agent execution failed", "error", err)
		return "", err
	}

	// Save messages to session
	if agentCfg.Memory.MaxMessages > 0 {
		// Check if this is a new session BEFORE saving (file existence = first exchange indicator)
		isNewSession := !o.SessionStore.SessionExists(userID, sessionID)
		if o.Debug {
			o.Logger.Debug("session check", "is_new", isNewSession, "session", sessionID)
		}

		// Save user message and assistant response
		_ = o.SessionStore.Save(userID, sessionID, "user", content)
		_ = o.SessionStore.Save(userID, sessionID, "assistant", response)

		// Auto-generate title after first exchange (async, non-blocking)
		if isNewSession {
			if o.Debug {
				o.Logger.Debug("launching title generation", "session", sessionID)
			}
			// Pass messages directly to avoid race condition with file I/O
			go o.generateTitle(agentCfg, userID, sessionID, content, response)
		}
	}

	return response, nil
}

// generateTitle generates a session title in the background
// Only called after first exchange (file didn't exist before HandleMessage)
func (o *Orchestrator) generateTitle(cfg *config.AgentConfig, userID, sessionID, userMsg, assistantMsg string) {
	if o.Debug {
		o.Logger.Debug("title generation started", "session", sessionID)
	}
	// Generate title (errors logged internally)
	if err := session.GenerateTitle(o.SessionStore, cfg, o.AgentFolder, userID, sessionID, userMsg, assistantMsg, o.Logger); err != nil {
		o.Logger.Warn("title generation failed", "error", err)
	}
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
