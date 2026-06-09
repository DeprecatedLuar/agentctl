package memory

import (
	"database/sql"

	"github.com/DeprecatedLuar/agentctl/internal/agent"
	_ "github.com/mattn/go-sqlite3"
)

// Message is an alias for agent.Message
type Message = agent.Message

const schema = `
CREATE TABLE IF NOT EXISTS messages (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    session_key TEXT NOT NULL,
    role        TEXT NOT NULL,
    content     TEXT NOT NULL,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_session ON messages(session_key, created_at);
`

// InitDB creates tables and indexes
func InitDB(db *sql.DB) error {
	_, err := db.Exec(schema)
	return err
}

// Load retrieves messages from memory in chronological order
func Load(db *sql.DB, sessionKey string, limit int) ([]Message, error) {
	// Query in DESC order, then reverse to get chronological
	query := `
		SELECT role, content
		FROM messages
		WHERE session_key = ?
		ORDER BY created_at DESC
		LIMIT ?
	`

	rows, err := db.Query(query, sessionKey, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		if err := rows.Scan(&msg.Role, &msg.Content); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Reverse to chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// Save stores a message in memory
func Save(db *sql.DB, sessionKey string, role string, content string) error {
	query := `
		INSERT INTO messages (session_key, role, content)
		VALUES (?, ?, ?)
	`
	_, err := db.Exec(query, sessionKey, role, content)
	return err
}
