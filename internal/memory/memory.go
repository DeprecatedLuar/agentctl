package memory

import (
	"database/sql"

	"github.com/DeprecatedLuar/agentctl/internal/agent"
)

// Message is an alias for agent.Message
type Message = agent.Message

// Load retrieves messages from memory (stub for Phase 5)
func Load(db *sql.DB, sessionKey string, limit int) ([]Message, error) {
	// TODO: Implement in Phase 5
	return []Message{}, nil
}

// Save stores a message in memory (stub for Phase 5)
func Save(db *sql.DB, sessionKey string, role string, content string) error {
	// TODO: Implement in Phase 5
	return nil
}
