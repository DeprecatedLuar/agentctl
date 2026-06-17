package internal

import (
	"context"
)

// MessageOptions contains all parameters for message handling including delivery options
type MessageOptions struct {
	// Basic message info
	Interface   string
	ContactID   string
	DisplayName string
	Username    string // Optional: @handle (e.g., Telegram @username)
	Content     string

	// Explicit resolution (CLI only)
	UserID    string
	SessionID string

	// Delivery options
	Channels       []string // --channel: deliver to these channels (no injection)
	ChannelsInject []string // --channel-inject: deliver and inject into session
	Tools          []string // --tools: whitelist of tools for this run
}

// MessageHandler processes incoming messages and returns responses
// Implemented by the orchestration layer, injected into interfaces
type MessageHandler interface {
	// HandleMessage processes a message with automatic contact resolution
	HandleMessage(iface, contactID, displayName, username, content string) (string, error)

	// HandleExplicitMessage processes a message with explicit user/session IDs
	// Used only by CLI interface for --user/--session flags (bypasses contact resolution)
	HandleExplicitMessage(userID, sessionID, iface, content string) (string, error)

	// HandleMessageWithOptions processes a message with full delivery options
	// Used when channel delivery or tool whitelisting is needed
	HandleMessageWithOptions(opts MessageOptions) (string, error)
}

// Dispatcher abstracts response delivery mechanism (socket, bot API, etc.)
// DEPRECATED: Legacy single-response dispatcher
type Dispatcher interface {
	Send(response string) error
}

// OutboundDispatcher handles cross-interface message delivery
type OutboundDispatcher interface {
	Send(iface, platformID, content string) error
	Register(sender interface{}) // Generic to avoid circular import
}

// Interface defines the contract for agent interfaces (CLI, Telegram, etc.)
// Interfaces handle I/O only - receiving input and delivering output
type Interface interface {
	Start(ctx context.Context) error
}
