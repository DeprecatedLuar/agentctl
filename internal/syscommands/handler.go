package syscommands

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/DeprecatedLuar/agentctl/internal/config"
	"github.com/DeprecatedLuar/agentctl/internal/session"
)

// Parse parses a command string and returns a Command struct
// Returns error if input doesn't start with "/" or is invalid
func Parse(input string) (*Command, error) {
	input = strings.TrimSpace(input)

	if !strings.HasPrefix(input, "/") {
		return nil, fmt.Errorf("not a command (missing / prefix)")
	}

	// Remove leading /
	input = strings.TrimPrefix(input, "/")

	// Split into name and args
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty command")
	}

	return &Command{
		Name: parts[0],
		Args: parts[1:],
	}, nil
}

// NewSession creates a new session and auto-switches to it
// Returns session metadata and agent config info
func NewSession(userID, iface string, store session.SessionStore, agentFolder string) (CommandResult, error) {
	// Generate new session ID
	sessionID := session.NewSessionID()

	// Load agent config to get model/provider/memory info
	agentConfig, issues := config.LoadAgent(agentFolder)
	if agentConfig == nil {
		return CommandResult{}, fmt.Errorf("failed to load agent config: configuration errors")
	}
	// Check for errors in validation issues
	for _, issue := range issues {
		if issue.Type == config.IssueError {
			return CommandResult{}, fmt.Errorf("failed to load agent config: %s", issue.Message)
		}
	}

	// Auto-switch to new session
	if err := store.SetLast(userID, iface, sessionID); err != nil {
		return CommandResult{}, fmt.Errorf("failed to set last session: %w", err)
	}

	// Format memory limit
	memoryStr := "unlimited"
	if agentConfig.Memory.MaxMessages > 0 {
		memoryStr = strconv.Itoa(agentConfig.Memory.MaxMessages)
	}

	// Return config data
	return CommandResult{
		Type: ResultTypeNewSession,
		Data: map[string]string{
			"model":    agentConfig.Agent.Model,
			"provider": agentConfig.Agent.Provider,
			"memory":   memoryStr,
		},
	}, nil
}

// ListSessions lists all sessions for the user
// Returns sorted list with active session marked
func ListSessions(userID, iface string, store session.SessionStore) (CommandResult, error) {
	// Get all sessions
	sessions, err := store.ListSessions(userID)
	if err != nil {
		return CommandResult{}, fmt.Errorf("failed to list sessions: %w", err)
	}

	// Get current active session
	activeSessionID, err := store.GetLast(userID, iface)
	if err != nil {
		// If no active session, that's okay
		activeSessionID = ""
	}

	// Sort by CreatedAt in reverse chronological order (newest first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].CreatedAt > sessions[j].CreatedAt
	})

	// Build SessionInfo list with active marker
	sessionInfos := make([]SessionInfo, 0, len(sessions))
	for _, s := range sessions {
		sessionInfos = append(sessionInfos, SessionInfo{
			SessionID: s.SessionID,
			Title:     s.Title,
			CreatedAt: s.CreatedAt,
			IsActive:  false, // Will be set below
		})
	}

	// Mark active session
	for i := range sessionInfos {
		if sessionInfos[i].SessionID == activeSessionID {
			sessionInfos[i].IsActive = true
			break
		}
	}

	return CommandResult{
		Type: ResultTypeSessionList,
		Data: sessionInfos,
	}, nil
}

// SwitchSession switches to a session by number or ID
// Accepts either a numeric index (1-based) or a literal session ID
func SwitchSession(sessionArg, userID, iface string, store session.SessionStore) (CommandResult, error) {
	if sessionArg == "" {
		return CommandResult{}, fmt.Errorf("session argument required (number or session ID)")
	}

	var targetSessionID string

	// Check if arg is a number (session index)
	if num, err := strconv.Atoi(sessionArg); err == nil {
		// Resolve by index (1-based)
		sessions, err := store.ListSessions(userID)
		if err != nil {
			return CommandResult{}, fmt.Errorf("failed to list sessions: %w", err)
		}

		// Sort by CreatedAt in reverse chronological order (newest first)
		sort.Slice(sessions, func(i, j int) bool {
			return sessions[i].CreatedAt > sessions[j].CreatedAt
		})

		// Validate index
		if num < 1 || num > len(sessions) {
			return CommandResult{}, fmt.Errorf("invalid session number: %d (must be 1-%d)", num, len(sessions))
		}

		// Get session ID (convert 1-based to 0-based index)
		targetSessionID = sessions[num-1].SessionID
	} else {
		// Treat as literal session ID
		targetSessionID = sessionArg
	}

	// Verify session exists
	if !store.SessionExists(userID, targetSessionID) {
		return CommandResult{}, fmt.Errorf("session not found: %s", targetSessionID)
	}

	// Switch to session
	if err := store.SetLast(userID, iface, targetSessionID); err != nil {
		return CommandResult{}, fmt.Errorf("failed to switch session: %w", err)
	}

	// Get session metadata
	meta, err := store.GetMeta(userID, targetSessionID)
	if err != nil {
		return CommandResult{}, fmt.Errorf("failed to get session metadata: %w", err)
	}

	return CommandResult{
		Type: ResultTypeSessionSwitched,
		Data: SessionInfo{
			SessionID: targetSessionID,
			Title:     meta.Title,
			CreatedAt: meta.CreatedAt,
			IsActive:  true,
		},
	}, nil
}
