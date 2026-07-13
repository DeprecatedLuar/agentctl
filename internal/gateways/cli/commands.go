package cli

import (
	"fmt"

	"github.com/DeprecatedLuar/agentctl/internal/gateways"
	"github.com/DeprecatedLuar/agentctl/internal/session"
	"github.com/DeprecatedLuar/agentctl/internal/syscommands"
)

// GatewayName identifies the cli access/identity tier. There is no
// persistent cli gateway (no socket listener) - one-shot chat calls the
// MessageHandler directly - but the string is still the identity used for
// contact resolution, access control, and session ownership, same as any
// other gateway name.
const GatewayName = "cli"

// HandleCommand processes a parsed syscommand (e.g. /new, /sessions) and
// returns CLI-formatted output. username is the OS user driving the CLI.
func HandleCommand(store session.SessionStore, agentFolder, username string, cmd *syscommands.Command) (string, error) {
	userID, err := store.ResolveUser(GatewayName, username)
	if err != nil {
		return "", fmt.Errorf("failed to resolve user: %w", err)
	}

	switch cmd.Name {
	case "new":
		result, err := syscommands.NewSession(userID, GatewayName, store, agentFolder)
		if err != nil {
			return "", err
		}
		return formatNewSession(result), nil

	case "sessions":
		// Check for "attach" subcommand
		if len(cmd.Args) > 0 && cmd.Args[0] == "attach" {
			if len(cmd.Args) < 2 {
				return "", fmt.Errorf("/sessions attach requires a session number or ID")
			}
			sessionArg := cmd.Args[1]
			result, err := syscommands.SwitchSession(sessionArg, userID, GatewayName, store)
			if err != nil {
				return "", err
			}
			return formatSessionSwitched(result), nil
		}
		// List sessions
		result, err := syscommands.ListSessions(userID, GatewayName, store)
		if err != nil {
			return "", err
		}
		return formatSessionList(result), nil

	default:
		return "", fmt.Errorf("unknown command: /%s", cmd.Name)
	}
}

// formatNewSession formats /new command result for CLI
func formatNewSession(result syscommands.CommandResult) string {
	return gateways.FormatNewSession(result)
}

// formatSessionList formats /sessions command result for CLI (numbered list)
func formatSessionList(result syscommands.CommandResult) string {
	sessions := result.Data.([]syscommands.SessionInfo)
	return gateways.FormatSessionList(sessions, true)
}

// formatSessionSwitched formats session switch result for CLI
func formatSessionSwitched(result syscommands.CommandResult) string {
	sessionInfo := result.Data.(syscommands.SessionInfo)
	title := sessionInfo.Title
	if title == "" {
		title = "(untitled)"
	}
	return fmt.Sprintf("Switched to session: %s", title)
}
