package telegram

import (
	"fmt"

	"github.com/DeprecatedLuar/agentctl/internal/interfaces"
	"github.com/DeprecatedLuar/agentctl/internal/syscommands"
)

// handleCommand processes commands and returns Telegram-formatted output
func (t *TelegramInterface) handleCommand(cmd *syscommands.Command, contactID string) (string, error) {
	// Resolve user ID
	userID, err := t.store.ResolveUser(interfaceNameTelegram, contactID)
	if err != nil {
		return "", fmt.Errorf("failed to resolve user: %w", err)
	}

	switch cmd.Name {
	case "start":
		return telegramStartMessage, nil

	case "new":
		result, err := syscommands.NewSession(userID, interfaceNameTelegram, t.store, t.agentFolder)
		if err != nil {
			return "", err
		}
		return formatNewSession(result), nil

	case "sessions":
		// Telegram doesn't support numbered switching yet
		// Future: return inline keyboard buttons
		if len(cmd.Args) > 0 && cmd.Args[0] == "attach" {
			return "", fmt.Errorf("/sessions attach not supported on Telegram (use numbered list on CLI)")
		}
		// List sessions
		result, err := syscommands.ListSessions(userID, interfaceNameTelegram, t.store)
		if err != nil {
			return "", err
		}
		return formatTelegramSessionList(result), nil

	default:
		return "", fmt.Errorf("unknown command: /%s", cmd.Name)
	}
}

// formatNewSession formats /new command result for Telegram
func formatNewSession(result syscommands.CommandResult) string {
	return interfaces.FormatNewSession(result)
}

// formatTelegramSessionList formats /sessions command result for Telegram (plain list)
func formatTelegramSessionList(result syscommands.CommandResult) string {
	sessions := result.Data.([]syscommands.SessionInfo)
	return interfaces.FormatSessionList(sessions, false)
}
