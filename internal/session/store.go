package session

// Message represents a stored message in the session history
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	TS      int64  `json:"ts"`
}

// SessionMeta contains metadata about a session
type SessionMeta struct {
	SessionID string `json:"session_id"` // Session identifier (populated by ListSessions from filename)
	Title     string `json:"title"`
	CreatedAt int64  `json:"created_at"`
	Interface string `json:"interface"`
	UserID    string `json:"user_id"`
}

// SessionStore is the storage abstraction for session data
type SessionStore interface {
	// Load retrieves the last N messages from a session
	Load(userID, sessionID string, limit int) ([]Message, error)

	// Save appends a message to a session
	Save(userID, sessionID, role, content string) error

	// GetMeta retrieves session metadata
	GetMeta(userID, sessionID string) (SessionMeta, error)

	// SetMeta updates session metadata (atomic write)
	SetMeta(userID, sessionID string, meta SessionMeta) error

	// GetLast returns the last session ID for this user+interface
	GetLast(userID, iface string) (string, error)

	// SetLast updates the last session ID for this user+interface (atomic write)
	SetLast(userID, iface, sessionID string) error

	// ListSessions returns all session metadata for a user
	ListSessions(userID string) ([]SessionMeta, error)

	// EnsureContact logs a contact if not already present
	EnsureContact(iface, platformID, displayName, username string) error

	// ResolveUser returns userID for a given interface+platformID
	ResolveUser(iface, platformID string) (string, error)

	// SessionExists checks if a session file exists
	SessionExists(userID, sessionID string) bool

	// CreateSession ensures a session file exists on disk with its metadata
	// line, without appending any message. No-op if the file already exists.
	CreateSession(userID, sessionID string) error
}
