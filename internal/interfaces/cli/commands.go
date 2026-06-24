package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/DeprecatedLuar/agentctl/internal/syscommands"
)

// handleCommand processes commands and returns CLI-formatted output
func (c *CLIInterface) handleCommand(cmd *syscommands.Command) (string, error) {
	// Resolve user ID
	userID, err := c.store.ResolveUser(interfaceNameCLI, c.username)
	if err != nil {
		return "", fmt.Errorf("failed to resolve user: %w", err)
	}

	switch cmd.Name {
	case "new":
		result, err := syscommands.NewSession(userID, interfaceNameCLI, c.store, c.agentFolder)
		if err != nil {
			return "", err
		}
		return c.formatNewSession(result), nil

	case "sessions":
		// Check for "attach" subcommand
		if len(cmd.Args) > 0 && cmd.Args[0] == "attach" {
			if len(cmd.Args) < 2 {
				return "", fmt.Errorf("/sessions attach requires a session number or ID")
			}
			sessionArg := cmd.Args[1]
			result, err := syscommands.SwitchSession(sessionArg, userID, interfaceNameCLI, c.store)
			if err != nil {
				return "", err
			}
			return c.formatSessionSwitched(result), nil
		}
		// List sessions
		result, err := syscommands.ListSessions(userID, interfaceNameCLI, c.store)
		if err != nil {
			return "", err
		}
		return c.formatSessionList(result), nil

	default:
		return "", fmt.Errorf("unknown command: /%s", cmd.Name)
	}
}

// formatNewSession formats /new command result for CLI
func (c *CLIInterface) formatNewSession(result syscommands.CommandResult) string {
	data := result.Data.(map[string]string)
	return fmt.Sprintf("New session started\n\nModel: %s\nProvider: %s\nMemory: %s messages",
		data["model"], data["provider"], data["memory"])
}

// formatSessionList formats /sessions command result for CLI (numbered list)
func (c *CLIInterface) formatSessionList(result syscommands.CommandResult) string {
	sessions := result.Data.([]syscommands.SessionInfo)
	if len(sessions) == 0 {
		return "No sessions found"
	}

	var b strings.Builder
	b.WriteString("Sessions:\n")
	for i, s := range sessions {
		title := s.Title
		if title == "" {
			title = "(untitled)"
		}

		// Format date from timestamp
		date := formatTimestamp(s.CreatedAt)

		// Build line: "1. Title (date) [active]"
		b.WriteString(fmt.Sprintf("%d. %s (%s)", i+1, title, date))
		if s.IsActive {
			b.WriteString(" [active]")
		}
		b.WriteString("\n")
	}

	return strings.TrimSpace(b.String())
}

// formatSessionSwitched formats session switch result for CLI
func (c *CLIInterface) formatSessionSwitched(result syscommands.CommandResult) string {
	sessionInfo := result.Data.(syscommands.SessionInfo)
	title := sessionInfo.Title
	if title == "" {
		title = "(untitled)"
	}
	return fmt.Sprintf("Switched to session: %s", title)
}

// formatTimestamp converts Unix timestamp to YYYY-MM-DD format
func formatTimestamp(ts int64) string {
	if ts == 0 {
		return "unknown"
	}
	t := time.Unix(ts, 0)
	return t.Format("2006-01-02")
}
