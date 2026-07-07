package commands

import (
	"fmt"

	"github.com/DeprecatedLuar/agentctl/internal/message"
	"github.com/DeprecatedLuar/agentctl/internal/registry"
	"github.com/DeprecatedLuar/agentctl/internal/session"
)

func HandleInject(args []string) error {
	// Parse arguments
	path := "."
	role := ""
	sessionID := ""
	content := ""
	contentGiven := false

	i := 0
	for i < len(args) {
		switch args[i] {
		case flagAgent, flagAgentS:
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a path or name", flagAgent)
			}
			path = args[i+1]
			i += 2
		case "--role":
			if i+1 >= len(args) {
				return fmt.Errorf("--role requires a value (assistant or user)")
			}
			role = args[i+1]
			i += 2
		case flagSession, flagSessionS:
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires an id", flagSession)
			}
			sessionID = args[i+1]
			i += 2
		case flagMessage, flagMessageS:
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires text", flagMessage)
			}
			content = args[i+1]
			contentGiven = true
			i += 2
		default:
			return fmt.Errorf("unexpected argument: %s", args[i])
		}
	}

	// Validate required fields
	content, err := message.Resolve(content, contentGiven)
	if err != nil {
		return err
	}
	if role == "" {
		return fmt.Errorf("--role is required")
	}
	if sessionID == "" {
		return fmt.Errorf("--session is required")
	}

	// Validate role value
	if role != "assistant" && role != "user" {
		return fmt.Errorf("--role must be 'assistant' or 'user', got: %s", role)
	}

	// Resolve agent path
	absPath, err := registry.ResolveAgentPath(path)
	if err != nil {
		return err
	}

	// Create session store
	store := session.NewJSONLStore(absPath)

	// Find session owner by scanning all user folders
	resolved, err := session.ResolveExplicit(store, absPath, "", sessionID, "cli")
	if err != nil {
		return fmt.Errorf("failed to find session: %w", err)
	}

	// Inject turn
	if err := session.InjectTurn(store, resolved.UserID, resolved.SessionID, role, content); err != nil {
		return err
	}

	fmt.Printf("Injected %s turn into session %s/%s\n", role, resolved.UserID, resolved.SessionID)
	return nil
}
