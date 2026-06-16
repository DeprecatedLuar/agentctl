package service

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/DeprecatedLuar/agentctl/internal/agent"
	"github.com/DeprecatedLuar/agentctl/internal/config"
	"github.com/DeprecatedLuar/agentctl/internal/interfaces"
	"github.com/DeprecatedLuar/agentctl/internal/session"
)

// MessageOptions is an alias for interfaces.MessageOptions for convenience
type MessageOptions = interfaces.MessageOptions

// ConversationService orchestrates a single conversation turn
type ConversationService struct {
	AgentFolder  string
	SessionStore session.SessionStore
	Logger       *slog.Logger
	Verbose      bool
	Debug        bool
}

// HandleMessage implements the MessageHandler interface
// Executes a conversation turn with full orchestration:
// - Loads config/tools/prompt (hot reload)
// - Loads session history
// - Calls agent
// - Saves messages
// - Triggers title generation (async, first turn only)
func (s *ConversationService) HandleMessage(opts MessageOptions) (string, error) {
	// Load config, tools, and prompt fresh on each request (hot reload)
	agentCfg, agentIssues := config.LoadAgent(s.AgentFolder)
	if agentCfg == nil {
		err := formatValidationError("agent configuration", agentIssues)
		s.Logger.Error("failed to load agent config", "error", err)
		return "", err
	}

	tools, toolIssues := config.LoadTools(s.AgentFolder, agentCfg.Tools)
	prompt, promptIssues := config.Parse(s.AgentFolder, map[string]string{})

	// Collect all validation issues
	allIssues := append(agentIssues, toolIssues...)
	allIssues = append(allIssues, promptIssues...)

	// Check for errors
	for _, issue := range allIssues {
		if issue.Type == config.IssueError {
			err := formatValidationError("configuration", allIssues)
			s.Logger.Error("configuration validation failed", "error", err)
			return "", err
		}
	}

	// Load history from session
	var history []agent.Message
	if agentCfg.Memory.MaxMessages > 0 {
		messages, err := s.SessionStore.Load(opts.UserID, opts.SessionID, agentCfg.Memory.MaxMessages)
		if err != nil {
			s.Logger.Error("failed to load session history", "error", err)
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

	// Build agent input
	input := agent.Input{
		UserID:    opts.UserID,
		SessionID: opts.SessionID,
		Interface: opts.Interface,
		Content:   opts.Content,
	}

	// Call agent
	response, err := agent.Run(agentCfg, tools, prompt, history, input, s.AgentFolder, s.Logger, s.Verbose, s.Debug)
	if err != nil {
		s.Logger.Error("agent execution failed", "error", err)
		return "", err
	}

	// Save messages to session
	if agentCfg.Memory.MaxMessages > 0 {
		// Check if this is a new session BEFORE saving (file existence = first exchange indicator)
		isNewSession := !s.SessionStore.SessionExists(opts.UserID, opts.SessionID)
		if s.Debug {
			s.Logger.Debug("session check", "is_new", isNewSession, "session", opts.SessionID)
		}

		// Save user message and assistant response
		_ = s.SessionStore.Save(opts.UserID, opts.SessionID, "user", opts.Content)
		_ = s.SessionStore.Save(opts.UserID, opts.SessionID, "assistant", response)

		// Auto-generate title after first exchange (async, non-blocking)
		if isNewSession {
			if s.Debug {
				s.Logger.Debug("launching title generation", "session", opts.SessionID)
			}
			// Pass messages directly to avoid race condition with file I/O
			go s.generateTitle(agentCfg, opts.UserID, opts.SessionID, opts.Content, response)
		}
	}

	return response, nil
}

// generateTitle generates a session title in the background
// Only called after first exchange (file didn't exist before HandleMessage)
func (s *ConversationService) generateTitle(cfg *config.AgentConfig, userID, sessionID, userMsg, assistantMsg string) {
	if s.Debug {
		s.Logger.Debug("title generation started", "session", sessionID)
	}
	// Generate title (errors logged internally)
	if err := session.GenerateTitle(s.SessionStore, cfg, s.AgentFolder, userID, sessionID, userMsg, assistantMsg, s.Logger); err != nil {
		s.Logger.Warn("title generation failed", "error", err)
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
