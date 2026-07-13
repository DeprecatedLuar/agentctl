package session

import "fmt"

// InjectTurn writes a single turn to the session JSONL.
// Returns error if session does not exist.
// Used by both HandleInject command and chat's --deliver/--inject flags.
// Holds the same cross-process session lock a normal turn does, so an
// inject can't interleave with a concurrent turn for the same session.
func InjectTurn(agentFolder string, store SessionStore, userID, sessionID, role, content string) error {
	unlock, err := LockSession(agentFolder, userID, sessionID, nil)
	if err != nil {
		return fmt.Errorf("failed to lock session: %w", err)
	}
	defer unlock()

	// Check if session exists
	if !store.SessionExists(userID, sessionID) {
		return fmt.Errorf("session does not exist: %s/%s", userID, sessionID)
	}

	// Save the turn
	if err := store.Save(userID, sessionID, role, content); err != nil {
		return fmt.Errorf("failed to inject turn: %w", err)
	}

	return nil
}
