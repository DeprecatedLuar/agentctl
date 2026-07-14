package internal

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/DeprecatedLuar/agentctl/internal/agent"
	"github.com/DeprecatedLuar/agentctl/internal/config"
	"github.com/DeprecatedLuar/agentctl/internal/hooks"
	"github.com/DeprecatedLuar/agentctl/internal/queue"
	"github.com/DeprecatedLuar/agentctl/internal/resolution"
	"github.com/DeprecatedLuar/agentctl/internal/session"
)

// Orchestrator orchestrates a single conversation turn
type Orchestrator struct {
	AgentFolder  string
	SessionStore session.SessionStore
	Dispatcher   OutboundDispatcher // For cross-gateway message delivery
	Logger       *slog.Logger
	Verbose      bool
	Debug        bool
	Overrides    config.Overrides // Applied on top of agent.toml after every load; survives hot-reload
	queues       queue.Registry   // per-session message queues, keyed by userID+sessionID
	titleWG      sync.WaitGroup   // tracks in-flight async title generation
}

// Wait blocks until any in-flight async title generation completes. One-shot
// callers (chat/deliver) must call this before exiting, since the process
// would otherwise end before the background goroutine finishes; `serve`
// doesn't need it (the daemon keeps running).
func (o *Orchestrator) Wait() {
	o.titleWG.Wait()
}

// enqueue serializes content through the per-session queue for
// (userID, sessionID), creating the queue on first use, and blocks until the
// resulting turn has been processed. The in-process queue already serializes
// same-process callers; LockSession extends that guarantee across processes
// (e.g. a `serve` daemon and a one-shot `chat` hitting the same session).
func (o *Orchestrator) enqueue(userID, sessionID, gateway, content string, toolWhitelist []string) (string, error) {
	sq := o.queues.GetOrCreate(userID, sessionID, func(c string) (string, error) {
		unlock, err := session.LockSession(o.AgentFolder, userID, sessionID, o.Logger)
		if err != nil {
			return "", fmt.Errorf("failed to lock session: %w", err)
		}
		defer unlock()
		return o.handleMessage(userID, sessionID, gateway, c, toolWhitelist)
	})
	return sq.Enqueue(content)
}

// HandleMessage implements the MessageHandler interface
// Executes a conversation turn with full orchestration:
// - Resolves session from contact ID
// - Loads config/tools/prompt (hot reload)
// - Loads session history
// - Calls agent
// - Saves messages
// - Triggers title generation (async, first turn only)
func (o *Orchestrator) HandleMessage(gateway, contactID, displayName, username, content string) (string, error) {
	// Resolve session from contact ID
	resolved, err := session.Resolve(o.SessionStore, o.AgentFolder, gateway, contactID, displayName, username)
	if err != nil {
		o.Logger.Error("session resolution failed", "contact", contactID, "error", err)
		return "", fmt.Errorf("session resolution failed: %w", err)
	}

	// Check access control
	allowed, err := session.CheckAccess(o.AgentFolder, gateway, contactID)
	if err != nil {
		o.Logger.Error("access check failed", "contact", contactID, "error", err)
		return "", fmt.Errorf("access check failed: %w", err)
	}
	if !allowed {
		o.Logger.Warn("access denied", "contact", contactID, "gateway", gateway)
		return "", fmt.Errorf("access denied")
	}

	return o.enqueue(resolved.UserID, resolved.SessionID, gateway, content, nil)
}

// HandleExplicitMessage implements the MessageHandler interface for explicit resolution
// Used by the CLI gateway when --user/--session flags are provided
// Bypasses contact resolution and uses provided IDs directly
func (o *Orchestrator) HandleExplicitMessage(userID, sessionID, gateway, content string) (string, error) {
	// Reverse lookup platformID for access check
	platformID, err := session.LookupPlatformID(o.AgentFolder, userID, gateway)
	if err != nil {
		o.Logger.Error("platform lookup failed", "user", userID, "gateway", gateway, "error", err)
		return "", fmt.Errorf("platform lookup failed: %w", err)
	}

	// Check access control
	allowed, err := session.CheckAccess(o.AgentFolder, gateway, platformID)
	if err != nil {
		o.Logger.Error("access check failed", "user", userID, "error", err)
		return "", fmt.Errorf("access check failed: %w", err)
	}
	if !allowed {
		o.Logger.Warn("access denied", "user", userID, "gateway", gateway)
		return "", fmt.Errorf("access denied")
	}

	return o.enqueue(userID, sessionID, gateway, content, nil)
}

// handleMessage contains the core message handling logic, shared by every
// entry point (HandleMessage, HandleExplicitMessage, HandleMessageWithOptions):
// hooks -> resolve config/tools/prompt -> load history -> agent.Run -> save.
// toolWhitelist filters loaded tools down to the given names when non-empty;
// nil or empty applies no filtering.
func (o *Orchestrator) handleMessage(userID, sessionID, gateway, content string, toolWhitelist []string) (string, error) {

	// Execute prerun hook before config load. Model/provider aren't known yet
	// (config hasn't loaded), so the context passed here leaves them empty.
	prerunCtx := resolution.NewContext(o.AgentFolder, userID, "", sessionID, gateway, "", "", nil)
	if err := hooks.ExecutePrerun(o.AgentFolder, nil, resolution.SystemEnv(prerunCtx), o.Logger); err != nil {
		// Non-fatal, but log if something catastrophic happened
		o.Logger.Error("prerun hook error", "error", err)
	}

	// Load config, tools, and prompt fresh on each request (hot reload)
	agentCfg, agentIssues := config.LoadAgent(o.AgentFolder)
	if agentCfg == nil {
		err := formatValidationError("agent configuration", agentIssues)
		o.Logger.Error("failed to load agent config", "error", err)
		return "", err
	}
	o.Overrides.Apply(agentCfg)

	tools, toolIssues := config.LoadTools(o.AgentFolder, agentCfg.Agent.Tools)
	ctx := resolution.NewContext(
		o.AgentFolder,
		userID,
		"", // username not available at this layer
		sessionID,
		gateway,
		agentCfg.Agent.Model,
		agentCfg.Agent.Provider,
		agentCfg.Environment,
	)
	prompt, promptIssues := config.Parse(o.AgentFolder, ctx)

	// Apply tool whitelist if specified
	if len(toolWhitelist) > 0 {
		tools = filterTools(tools, toolWhitelist)
		if o.Debug {
			o.Logger.Debug("applied tool whitelist", "count", len(tools), "whitelist", toolWhitelist)
		}
	}

	// Collect and validate all issues
	allIssues := append(agentIssues, toolIssues...)
	allIssues = append(allIssues, promptIssues...)
	if err := checkIssues("configuration", allIssues); err != nil {
		o.Logger.Error("configuration validation failed", "error", err)
		return "", err
	}

	// Load history from session
	var history []agent.Message
	if agentCfg.Memory.HistoryEnabled() {
		messages, err := o.SessionStore.Load(userID, sessionID, agentCfg.Memory.MaxMessages)
		if err != nil {
			o.Logger.Error("failed to load session history", "error", err)
			return "", err
		}
		history = toAgentMessages(messages)
	}

	// Build agent input
	input := agent.Input{
		UserID:    userID,
		SessionID: sessionID,
		Gateway:   gateway,
		Content:   content,
	}

	// Build a callback to deliver intermediate narration turns (text
	// alongside tool calls) back to the originating gateway/contact as
	// soon as they happen, instead of only the final no-tool-call reply.
	var onPartial func(string)
	if o.Dispatcher != nil {
		if platformID, perr := session.LookupPlatformID(o.AgentFolder, userID, gateway); perr == nil {
			onPartial = func(text string) {
				if err := o.Dispatcher.Send(gateway, platformID, text); err != nil {
					o.Logger.Warn("partial delivery failed", "gateway", gateway, "error", err)
				}
			}
		} else {
			o.Logger.Warn("partial delivery lookup failed", "error", perr)
		}
	}

	// Call agent
	response, err := agent.Run(agentCfg, tools, prompt, history, input, o.AgentFolder, o.Logger, o.Verbose, o.Debug, onPartial)
	if err != nil {
		o.Logger.Error("agent execution failed", "error", err)
		return "", err
	}

	// Save messages to session and kick off title generation if needed
	o.saveAndMaybeTitle(agentCfg, userID, sessionID, content, response)

	return response, nil
}

// checkIssues returns a formatted error if issues contains anything severe
// enough to block execution (see config.HasBlockingError), or nil otherwise.
func checkIssues(context string, issues []config.ValidationIssue) error {
	if !config.HasBlockingError(issues) {
		return nil
	}
	return formatValidationError(context, issues)
}

// toAgentMessages converts stored session messages to the format agent.Run expects.
func toAgentMessages(msgs []session.Message) []agent.Message {
	history := make([]agent.Message, len(msgs))
	for i, msg := range msgs {
		history[i] = agent.Message{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}
	return history
}

// saveAndMaybeTitle persists the user/assistant turn to the session (when
// memory is enabled) and, on the session's first real exchange, kicks off
// async title generation.
func (o *Orchestrator) saveAndMaybeTitle(agentCfg *config.AgentConfig, userID, sessionID, content, response string) {
	if !agentCfg.Memory.HistoryEnabled() {
		return
	}

	// Check BEFORE saving whether a title is still needed.
	meta, err := o.SessionStore.GetMeta(userID, sessionID)
	if err != nil {
		o.Logger.Warn("failed to get session metadata", "session", sessionID, "error", err)
	}
	needsTitle := meta.NeedsTitle()
	if o.Debug {
		o.Logger.Debug("session check", "needs_title", needsTitle, "session", sessionID)
	}

	// Save user message and assistant response
	if err := o.SessionStore.Save(userID, sessionID, "user", content); err != nil {
		o.Logger.Error("failed to save user message", "session", sessionID, "error", err)
	}
	if err := o.SessionStore.Save(userID, sessionID, "assistant", response); err != nil {
		o.Logger.Error("failed to save assistant message", "session", sessionID, "error", err)
	}

	// Auto-generate title after first real exchange (async, non-blocking).
	// Tracked via titleWG so one-shot callers can drain it with Wait()
	// before the process exits.
	if needsTitle {
		if o.Debug {
			o.Logger.Debug("launching title generation", "session", sessionID)
		}
		// Pass messages directly to avoid race condition with file I/O
		o.titleWG.Go(func() {
			o.generateTitle(agentCfg, userID, sessionID, content, response)
		})
	}
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
		resolved, err := session.ResolveExplicit(o.SessionStore, o.AgentFolder, opts.UserID, opts.SessionID, opts.Gateway)
		if err != nil {
			o.Logger.Error("explicit session resolution failed", "user", opts.UserID, "session", opts.SessionID, "error", err)
			return "", fmt.Errorf("session resolution failed: %w", err)
		}
		userID = resolved.UserID
		sessionID = resolved.SessionID

		// Reverse lookup platformID for access check
		platformID, err := session.LookupPlatformID(o.AgentFolder, userID, opts.Gateway)
		if err != nil {
			o.Logger.Error("platform lookup failed", "user", userID, "gateway", opts.Gateway, "error", err)
			return "", fmt.Errorf("platform lookup failed: %w", err)
		}

		// Check access control
		allowed, err := session.CheckAccess(o.AgentFolder, opts.Gateway, platformID)
		if err != nil {
			o.Logger.Error("access check failed", "user", userID, "error", err)
			return "", fmt.Errorf("access check failed: %w", err)
		}
		if !allowed {
			o.Logger.Warn("access denied", "user", userID, "gateway", opts.Gateway)
			return "", fmt.Errorf("access denied")
		}
	} else {
		// Auto resolution (like HandleMessage)
		resolved, err := session.Resolve(o.SessionStore, o.AgentFolder, opts.Gateway, opts.ContactID, opts.DisplayName, opts.Username)
		if err != nil {
			o.Logger.Error("session resolution failed", "contact", opts.ContactID, "error", err)
			return "", fmt.Errorf("session resolution failed: %w", err)
		}
		userID = resolved.UserID
		sessionID = resolved.SessionID

		// Check access control
		allowed, err := session.CheckAccess(o.AgentFolder, opts.Gateway, opts.ContactID)
		if err != nil {
			o.Logger.Error("access check failed", "contact", opts.ContactID, "error", err)
			return "", fmt.Errorf("access check failed: %w", err)
		}
		if !allowed {
			o.Logger.Warn("access denied", "contact", opts.ContactID, "gateway", opts.Gateway)
			return "", fmt.Errorf("access denied")
		}
	}

	// Run agent and get response, unless Raw delivers Content verbatim
	var response string
	if opts.Raw {
		response = opts.Content
	} else {
		response, err = o.enqueue(userID, sessionID, opts.Gateway, opts.Content, opts.Tools)
		if err != nil {
			return "", err
		}
	}

	// Handle channel delivery
	if len(opts.Deliver) > 0 {
		role := opts.Role
		if role == "" {
			role = "assistant"
		}
		if err := o.deliverToChannels(userID, opts.Deliver, opts.Inject, role, opts.Note, response); err != nil {
			o.Logger.Warn("channel delivery failed", "error", err)
			// Don't fail the whole request - user still gets their response
		}
	}

	return response, nil
}

// deliverToChannels delivers response to specified channels, optionally injecting into the target session.
// When inject and note are both set, a system-role turn with the note text is injected immediately
// before the response turn, so the model has context without the note ever reaching the channel.
func (o *Orchestrator) deliverToChannels(currentUserID string, channels []string, inject bool, role, note, response string) error {
	if o.Dispatcher == nil {
		return fmt.Errorf("dispatcher not configured")
	}

	for _, channelStr := range channels {
		userID, platformID, targetSessionID, err := session.ResolveChannel(o.SessionStore, o.AgentFolder, channelStr, currentUserID)
		if err != nil {
			o.Logger.Warn("failed to resolve channel", "channel", channelStr, "error", err)
			continue
		}

		// Extract gateway name from channel string
		gateway := channelStr
		if strings.Contains(channelStr, "@") {
			parts := strings.SplitN(channelStr, "@", 2)
			gateway = parts[1]
		}

		if err := o.Dispatcher.Send(gateway, platformID, response); err != nil {
			o.Logger.Warn("failed to deliver to channel", "channel", channelStr, "error", err)
			continue // Don't inject if delivery failed
		}
		if o.Debug {
			o.Logger.Debug("delivered to channel", "channel", channelStr, "user", userID)
		}

		if inject {
			if note != "" {
				if err := session.InjectTurn(o.AgentFolder, o.SessionStore, userID, targetSessionID, "system", note); err != nil {
					o.Logger.Warn("failed to inject system note", "channel", channelStr, "session", targetSessionID, "error", err)
				}
			}
			if err := session.InjectTurn(o.AgentFolder, o.SessionStore, userID, targetSessionID, role, response); err != nil {
				o.Logger.Warn("failed to inject turn", "channel", channelStr, "session", targetSessionID, "error", err)
			} else if o.Debug {
				o.Logger.Debug("injected", "channel", channelStr, "user", userID, "session", targetSessionID)
			}
		}
	}

	return nil
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
