package commands

import (
	"fmt"
	"os"
	"os/user"

	"github.com/DeprecatedLuar/agentctl/internal"
	"github.com/DeprecatedLuar/agentctl/internal/gateways/cli"
	"github.com/DeprecatedLuar/agentctl/internal/message"
	"github.com/DeprecatedLuar/agentctl/internal/registry"
)

// HandleDeliver sends literal text straight to one or more channels via a
// one-shot Orchestrator, bypassing the LLM entirely (unlike chat). No daemon
// is required.
func HandleDeliver(args []string) error {
	// Parse arguments
	path := "."
	userFlag := ""
	text := ""
	messageGiven := false
	inject := false
	role := ""
	note := ""
	var deliver []string

	rules := []flagRule{
		{names: []string{flagAgent, flagAgentS}, parse: func(args []string, i int) (int, error) {
			if i+1 >= len(args) {
				return 0, fmt.Errorf("%s requires a path or name", flagAgent)
			}
			path = args[i+1]
			return 2, nil
		}},
		{names: []string{flagUser, flagUserS}, parse: func(args []string, i int) (int, error) {
			if i+1 >= len(args) {
				return 0, fmt.Errorf("%s requires an id", flagUser)
			}
			userFlag = args[i+1]
			return 2, nil
		}},
		{names: []string{flagInject}, parse: func(args []string, i int) (int, error) {
			inject = true
			consumed, resolvedRole := parseInjectArg(args, i)
			role = resolvedRole
			return consumed, nil
		}},
		{names: []string{flagNote}, parse: func(args []string, i int) (int, error) {
			if i+1 >= len(args) {
				return 0, fmt.Errorf("%s requires text", flagNote)
			}
			note = args[i+1]
			return 2, nil
		}},
		{names: []string{flagMessage, flagMessageS}, parse: func(args []string, i int) (int, error) {
			if i+1 >= len(args) {
				return 0, fmt.Errorf("%s requires text", flagMessage)
			}
			text = args[i+1]
			messageGiven = true
			return 2, nil
		}},
	}

	if err := parseFlags(args, rules, func(arg string) error {
		if len(arg) > 0 && arg[0] == '-' {
			return fmt.Errorf("unexpected argument: %s", arg)
		}
		if len(deliver) > 0 {
			return fmt.Errorf("unexpected argument: %s", arg)
		}
		deliver = parseCommaSeparated(arg)
		return nil
	}); err != nil {
		return err
	}

	// Apply env vars (flags/args take priority)
	if userFlag == "" {
		userFlag = os.Getenv(envUser)
	}

	text, err := message.Resolve(text, messageGiven)
	if err != nil {
		return err
	}

	if len(deliver) == 0 {
		return fmt.Errorf("usage: agentctl deliver <channel[,channel...]> [--message text] [--inject [role]] [--note text] [--agent name|path] [--user id]")
	}
	if note != "" && !inject {
		return fmt.Errorf("%s requires %s", flagNote, flagInject)
	}

	// Resolve agent path
	absPath, err := registry.ResolveAgentPath(path)
	if err != nil {
		return err
	}

	_, lg, err := loadAgentAndLogger(absPath, false)
	if err != nil {
		return err
	}

	// Build only the Senders the requested channels need
	assembled, err := assembleOrchestrator(absPath, nil, lg, false, false, deliveryGateways(deliver))
	if err != nil {
		return err
	}

	// Mirrors the old socket-server's handleWithOptions: an explicit --user
	// resolves directly; otherwise fall back to the OS user as contact ID,
	// same identity a plain `chat` would use.
	currentUser, uerr := user.Current()
	username := "unknown"
	if uerr == nil {
		username = currentUser.Username
	}

	opts := internal.MessageOptions{
		Gateway:     cli.GatewayName,
		ContactID:   username,
		DisplayName: username,
		Content:     text,
		UserID:      userFlag,
		Deliver:     deliver,
		Inject:      inject,
		Role:        role,
		Note:        note,
		Raw:         true,
	}

	if _, err := assembled.Orchestrator.HandleMessageWithOptions(opts); err != nil {
		return fmt.Errorf("agent error: %w", err)
	}

	fmt.Printf("Delivered to: %v\n", deliver)
	return nil
}
