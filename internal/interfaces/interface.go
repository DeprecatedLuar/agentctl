package interfaces

import (
	"context"
)

// Interface defines the contract for agent interfaces (CLI, Telegram, etc.)
// Interfaces handle I/O only - receiving input and delivering output
type Interface interface {
	Start(ctx context.Context) error
}

// MessageHandler processes incoming messages and returns responses
// Implemented by the service layer, injected into interfaces
type MessageHandler interface {
	HandleMessage(opts MessageOptions) (string, error)
}

// MessageOptions contains all context needed to process a message
type MessageOptions struct {
	UserID    string
	SessionID string
	Interface string
	Content   string
}
