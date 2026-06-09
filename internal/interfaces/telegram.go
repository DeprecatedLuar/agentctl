package interfaces

import (
	"context"
	"fmt"
)

// TelegramInterface implements the Telegram bot interface
type TelegramInterface struct {
	agentFolder string
}

// NewTelegram creates a new Telegram interface
func NewTelegram(agentFolder string) *TelegramInterface {
	return &TelegramInterface{agentFolder: agentFolder}
}

// Start begins the Telegram bot polling loop
func (t *TelegramInterface) Start(ctx context.Context, runner *Runner) error {
	// TODO: Implement in Phase 6
	return fmt.Errorf("telegram interface not yet implemented")
}
