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
	Dispatcher   OutboundDispatcher // For cross-interface message delivery
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
func (o *Orchestrator) HandleMessage(iface, contactID, displayName, username, content string) (string, error) {
	// Resolve session from contact ID
	resolved, err := session.Resolve(o.SessionStore, o.AgentFolder, iface, contactID, displayName, username)
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

// HandleMessageWithOptions implements the MessageHandler interface for full delivery options
// Used when channel delivery or tool whitelisting is needed
func (o *Orchestrator) HandleMessageWithOptions(opts MessageOptions) (string, error) {
	var userID, sessionID string
	var err error

	// Determine resolution strategy
	if opts.UserID != "" || opts.SessionID != "" {
		// Explicit resolution (like HandleExplicitMessage)
		resolved, err := session.ResolveExplicit(o.SessionStore, o.AgentFolder, opts.UserID, opts.SessionID, opts.Interface)
		if err != nil {
			o.Logger.Error("explicit session resolution failed", "user", opts.UserID, "session", opts.SessionID, "error", err)
			return "", fmt.Errorf("session resolution failed: %w", err)
		}
		userID = resolved.UserID
		sessionID = resolved.SessionID
	} else {
		// Auto resolution (like HandleMessage)
		resolved, err := session.Resolve(o.SessionStore, o.AgentFolder, opts.Interface, opts.ContactID, opts.DisplayName, opts.Username)
		if err != nil {
			o.Logger.Error("session resolution failed", "contact", opts.ContactID, "error", err)
			return "", fmt.Errorf("session resolution failed: %w", err)
		}
		userID = resolved.UserID
		sessionID = resolved.SessionID
	}

	// Run agent and get response
	response, err := o.handleMessageInternalWithTools(userID, sessionID, opts.Interface, opts.Content, opts.Tools)
	if err != nil {
		return "", err
	}

	// Handle channel delivery
	if len(opts.Channels) > 0 || len(opts.ChannelsInject) > 0 {
		if err := o.deliverToChannels(userID, opts.Channels, opts.ChannelsInject, response); err != nil {
			o.Logger.Warn("channel delivery failed", "error", err)
			// Don't fail the whole request - user still gets their response
		}
	}

	return response, nil
}

// deliverToChannels delivers response to specified channels with optional injection
func (o *Orchestrator) deliverToChannels(currentUserID string, channels, channelsInject []string, response string) error {
	if o.Dispatcher == nil {
		return fmt.Errorf("dispatcher not configured")
	}

	// Process --channel (delivery only)
	for _, channelStr := range channels {
		userID, platformID, _, err := session.ResolveChannel(o.SessionStore, o.AgentFolder, channelStr, currentUserID)
		if err != nil {
			o.Logger.Warn("failed to resolve channel", "channel", channelStr, "error", err)
			continue
		}

		// Extract interface from channel string
		iface := channelStr
		if strings.Contains(channelStr, "@") {
			parts := strings.SplitN(channelStr, "@", 2)
			iface = parts[1]
		}

		if err := o.Dispatcher.Send(iface, platformID, response); err != nil {
			o.Logger.Warn("failed to deliver to channel", "channel", channelStr, "error", err)
		} else if o.Debug {
			o.Logger.Debug("delivered to channel", "channel", channelStr, "user", userID)
		}
	}

	// Process --channel-inject (delivery + injection)
	for _, channelStr := range channelsInject {
		userID, platformID, targetSessionID, err := session.ResolveChannel(o.SessionStore, o.AgentFolder, channelStr, currentUserID)
		if err != nil {
			o.Logger.Warn("failed to resolve channel", "channel", channelStr, "error", err)
			continue
		}

		// Extract interface from channel string
		iface := channelStr
		if strings.Contains(channelStr, "@") {
			parts := strings.SplitN(channelStr, "@", 2)
			iface = parts[1]
		}

		// Deliver message
		if err := o.Dispatcher.Send(iface, platformID, response); err != nil {
			o.Logger.Warn("failed to deliver to channel", "channel", channelStr, "error", err)
			continue // Don't inject if delivery failed
		}

		// Inject assistant turn into target session
		if err := session.InjectTurn(o.SessionStore, userID, targetSessionID, "assistant", response); err != nil {
			o.Logger.Warn("failed to inject turn", "channel", channelStr, "session", targetSessionID, "error", err)
		} else if o.Debug {
			o.Logger.Debug("delivered and injected", "channel", channelStr, "user", userID, "session", targetSessionID)
		}
	}

	return nil
}

// handleMessageInternalWithTools is like handleMessageInternal but supports tool whitelisting
func (o *Orchestrator) handleMessageInternalWithTools(userID, sessionID, iface, content string, toolWhitelist []string) (string, error) {
	// Load config, tools, and prompt fresh on each request (hot reload)
	agentCfg, agentIssues := config.LoadAgent(o.AgentFolder)
	if agentCfg == nil {
		err := formatValidationError("agent configuration", agentIssues)
		o.Logger.Error("failed to load agent config", "error", err)
		return "", err
	}

	tools, toolIssues := config.LoadTools(o.AgentFolder, agentCfg.Tools)
	prompt, promptIssues := config.Parse(o.AgentFolder, map[string]string{})

	// Apply tool whitelist if specified
	if len(toolWhitelist) > 0 {
		tools = filterTools(tools, toolWhitelist)
		if o.Debug {
			o.Logger.Debug("applied tool whitelist", "count", len(tools), "whitelist", toolWhitelist)
		}
	}

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

// filterTools returns only tools matching the whitelist
func filterTools(tools []config.ToolConfig, whitelist []string) []config.ToolConfig {
	whitelistMap := make(map[string]bool)
	for _, name := range whitelist {
		whitelistMap[name] = true
	}

	var filtered []config.ToolConfig
	for _, tool := range tools {
		if whitelistMap[tool.Name] {
			filtered = append(filtered, tool)
		}
	}
	return filtered
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
