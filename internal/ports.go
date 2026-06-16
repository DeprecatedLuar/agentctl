package internal

import (
	"context"
)

// MessageHandler processes incoming messages and returns responses
// Implemented by the orchestration layer, injected into interfaces
type MessageHandler interface {
	// HandleMessage processes a message with automatic contact resolution
	HandleMessage(iface, contactID, displayName, content string) (string, error)

	// HandleExplicitMessage processes a message with explicit user/session IDs
	// Used only by CLI interface for --user/--session flags (bypasses contact resolution)
	HandleExplicitMessage(userID, sessionID, iface, content string) (string, error)
}

// Dispatcher abstracts response delivery mechanism (socket, bot API, etc.)
type Dispatcher interface {
	Send(response string) error
}

// Interface defines the contract for agent interfaces (CLI, Telegram, etc.)
// Interfaces handle I/O only - receiving input and delivering output
type Interface interface {
	Start(ctx context.Context) error
}
