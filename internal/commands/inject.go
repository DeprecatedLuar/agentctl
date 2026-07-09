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

	rules := []flagRule{
		{names: []string{flagAgent, flagAgentS}, parse: func(args []string, i int) (int, error) {
			if i+1 >= len(args) {
				return 0, fmt.Errorf("%s requires a path or name", flagAgent)
			}
			path = args[i+1]
			return 2, nil
		}},
		{names: []string{flagSession, flagSessionS}, parse: func(args []string, i int) (int, error) {
			if i+1 >= len(args) {
				return 0, fmt.Errorf("%s requires an id", flagSession)
			}
			sessionID = args[i+1]
			return 2, nil
		}},
		{names: []string{flagMessage, flagMessageS}, parse: func(args []string, i int) (int, error) {
			if i+1 >= len(args) {
				return 0, fmt.Errorf("%s requires text", flagMessage)
			}
			content = args[i+1]
			contentGiven = true
			return 2, nil
		}},
	}

	if err := parseFlags(args, rules, func(arg string) error {
		if len(arg) > 0 && arg[0] == '-' {
			return fmt.Errorf("unexpected argument: %s", arg)
		}
		if role != "" {
			return fmt.Errorf("unexpected argument: %s", arg)
		}
		role = arg
		return nil
	}); err != nil {
		return err
	}

	// Validate required fields
	content, err := message.Resolve(content, contentGiven)
	if err != nil {
		return err
	}
	if sessionID == "" {
		return fmt.Errorf("--session is required")
	}

	// Role defaults to assistant when omitted
	if role == "" {
		role = roleAssistant
	}
	if !isValidRole(role) {
		return fmt.Errorf("role must be 'assistant', 'user', or 'system', got: %s", role)
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
