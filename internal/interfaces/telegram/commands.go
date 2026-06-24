package telegram

import (
	"fmt"
	"strings"
	"time"

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
	data := result.Data.(map[string]string)
	return fmt.Sprintf("New session started\n\nModel: %s\nProvider: %s\nMemory: %s messages",
		data["model"], data["provider"], data["memory"])
}

// formatTelegramSessionList formats /sessions command result for Telegram (plain list)
func formatTelegramSessionList(result syscommands.CommandResult) string {
	sessions := result.Data.([]syscommands.SessionInfo)
	if len(sessions) == 0 {
		return "No sessions found"
	}

	var b strings.Builder
	b.WriteString("Sessions:\n")
	for _, s := range sessions {
		title := s.Title
		if title == "" {
			title = "(untitled)"
		}

		// Format date from timestamp
		date := formatTelegramTimestamp(s.CreatedAt)

		// Build line: "- Title (date) [active]"
		b.WriteString(fmt.Sprintf("- %s (%s)", title, date))
		if s.IsActive {
			b.WriteString(" [active]")
		}
		b.WriteString("\n")
	}

	return strings.TrimSpace(b.String())
}

// formatTelegramTimestamp converts Unix timestamp to YYYY-MM-DD format
func formatTelegramTimestamp(ts int64) string {
	if ts == 0 {
		return "unknown"
	}
	t := time.Unix(ts, 0)
	return t.Format("2006-01-02")
}
