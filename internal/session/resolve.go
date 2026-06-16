package session

import (
	"fmt"
	"os"
	"path/filepath"
)

// Resolve is called by interfaces on every incoming message.
// 1. Lazy migration: migrate this contact if they have an unlinked folder
// 2. Calls ResolveUser to get userId
// 3. Calls EnsureContact to log the contact
// 4. Calls GetLast to find existing session or generates new SessionID
// 5. Calls SetLast to persist
// Returns ResolvedSession{UserID, SessionID}
func Resolve(store SessionStore, agentFolder, iface, platformID, displayName string) (ResolvedSession, error) {
	// Step 0: Lazy migration (best-effort, instant if no migration needed)
	_ = MigrateContact(agentFolder, iface, platformID) // Ignore errors - migration is best-effort

	// Step 1: Resolve user ID
	userID, err := store.ResolveUser(iface, platformID)
	if err != nil {
		return ResolvedSession{}, fmt.Errorf("failed to resolve user: %w", err)
	}

	// Step 2: Log contact
	if err := store.EnsureContact(iface, platformID, displayName); err != nil {
		return ResolvedSession{}, fmt.Errorf("failed to log contact: %w", err)
	}

	// Step 3: Get or create session ID
	sessionID, err := store.GetLast(userID, iface)
	if err != nil {
		return ResolvedSession{}, fmt.Errorf("failed to get last session: %w", err)
	}

	if sessionID == "" {
		// No existing session - generate new one
		sessionID = NewSessionID()
	}

	// Step 4: Persist as last session
	if err := store.SetLast(userID, iface, sessionID); err != nil {
		return ResolvedSession{}, fmt.Errorf("failed to set last session: %w", err)
	}

	return ResolvedSession{
		UserID:    userID,
		SessionID: sessionID,
	}, nil
}

// ResolveExplicit is used by chat command when --user/--session flags are set.
// If sessionID is empty, uses last session for that user+interface.
// If userID is empty, scans all user folders to find which owns the sessionID.
func ResolveExplicit(store SessionStore, agentFolder, userID, sessionID, iface string) (ResolvedSession, error) {
	// Case 1: Both specified
	if userID != "" && sessionID != "" {
		return ResolvedSession{
			UserID:    userID,
			SessionID: sessionID,
		}, nil
	}

	// Case 2: User specified, session empty - get last session
	if userID != "" && sessionID == "" {
		lastSessionID, err := store.GetLast(userID, iface)
		if err != nil {
			return ResolvedSession{}, fmt.Errorf("failed to get last session: %w", err)
		}

		if lastSessionID == "" {
			// No existing session - generate new one
			lastSessionID = NewSessionID()
			if err := store.SetLast(userID, iface, lastSessionID); err != nil {
				return ResolvedSession{}, fmt.Errorf("failed to set last session: %w", err)
			}
		}

		return ResolvedSession{
			UserID:    userID,
			SessionID: lastSessionID,
		}, nil
	}

	// Case 3: Session specified, user empty - find owner
	if userID == "" && sessionID != "" {
		foundUserID, err := findSessionOwner(agentFolder, sessionID)
		if err != nil {
			return ResolvedSession{}, err
		}

		return ResolvedSession{
			UserID:    foundUserID,
			SessionID: sessionID,
		}, nil
	}

	// Case 4: Both empty - error (shouldn't happen if called correctly)
	return ResolvedSession{}, fmt.Errorf("either userID or sessionID must be specified")
}

// findSessionOwner scans all user folders to find which one owns the given sessionID
func findSessionOwner(agentFolder, sessionID string) (string, error) {
	sessionsRoot := filepath.Join(agentFolder, sessionsDir)

	// Read all user directories
	entries, err := os.ReadDir(sessionsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("session '%s' not found (no sessions exist)", sessionID)
		}
		return "", fmt.Errorf("failed to read sessions directory: %w", err)
	}

	// Search for session file in each user directory
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		userID := entry.Name()
		sessionFile := filepath.Join(sessionsRoot, userID, sessionID+".jsonl")

		if _, err := os.Stat(sessionFile); err == nil {
			// Found it!
			return userID, nil
		}
	}

	return "", fmt.Errorf("session '%s' not found in any user folder", sessionID)
}
