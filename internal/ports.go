package internal

import (
	"context"

	"github.com/DeprecatedLuar/agentctl/internal/gateways"
)

// MessageOptions contains all parameters for message handling including delivery options
type MessageOptions struct {
	// Basic message info
	Gateway     string
	ContactID   string
	DisplayName string
	Username    string // Optional: @handle (e.g., Telegram @username)
	Content     string

	// Explicit resolution (CLI only)
	UserID    string
	SessionID string

	// Delivery options
	Deliver []string // --deliver: channels to deliver the response to
	Inject  bool     // --inject: also inject the delivered response into the target session
	Role    string   // role of the injected turn (assistant|user|system), defaults to "assistant"
	Note    string   // optional system-role note injected immediately before the delivered turn
	Tools   []string // --tools: whitelist of tools for this run
	Raw     bool     // true: skip the agent call, treat Content as the response verbatim (used by `deliver`)
}

// MessageHandler processes incoming messages and returns responses
// Implemented by the orchestration layer, injected into gateways
type MessageHandler interface {
	// HandleMessage processes a message with automatic contact resolution
	HandleMessage(gateway, contactID, displayName, username, content string) (string, error)

	// HandleExplicitMessage processes a message with explicit user/session IDs
	// Used only by CLI gateway for --user/--session flags (bypasses contact resolution)
	HandleExplicitMessage(userID, sessionID, gateway, content string) (string, error)

	// HandleMessageWithOptions processes a message with full delivery options
	// Used when channel delivery or tool whitelisting is needed
	HandleMessageWithOptions(opts MessageOptions) (string, error)
}

// OutboundDispatcher handles cross-gateway message delivery
type OutboundDispatcher interface {
	Send(gateway, platformID, content string) error
	Register(sender gateways.Sender)
}

// Gateway defines the contract for agent gateways (CLI, Telegram, etc.)
// Gateways handle I/O only - receiving input and delivering output
type Gateway interface {
	Start(ctx context.Context) error
}
