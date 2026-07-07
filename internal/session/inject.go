package session

import "fmt"

// InjectTurn writes a single turn to the session JSONL.
// Returns error if session does not exist.
// Used by both HandleInject command and chat's --deliver/--inject flags.
func InjectTurn(store SessionStore, userID, sessionID, role, content string) error {
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
