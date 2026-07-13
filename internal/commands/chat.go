package commands

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/DeprecatedLuar/agentctl/internal"
	"github.com/DeprecatedLuar/agentctl/internal/config"
	"github.com/DeprecatedLuar/agentctl/internal/gateways/cli"
	"github.com/DeprecatedLuar/agentctl/internal/message"
	"github.com/DeprecatedLuar/agentctl/internal/registry"
	"github.com/DeprecatedLuar/agentctl/internal/session"
	"github.com/DeprecatedLuar/agentctl/internal/syscommands"
)

const (
	// Flags
	flagAgent    = "--agent"
	flagAgentS   = "-a"
	flagUser     = "--user"
	flagUserS    = "-u"
	flagSession  = "--session"
	flagSessionS = "-s"
	flagDeliver  = "--deliver"
	flagInject   = "--inject"
	flagNote     = "--note"
	flagTools    = "--tools"
	flagMessage  = "--message"
	flagMessageS = "-m"
	flagModel    = "--model"
	flagProvider = "--provider"
	// flagDebug is defined in serve.go and shared across commands

	// Environment variables
	envAgent    = "AGENTCTL_AGENT"
	envUser     = "AGENTCTL_USER"
	envSession  = "AGENTCTL_SESSION"
	envModel    = "AGENTCTL_MODEL"
	envProvider = "AGENTCTL_PROVIDER"
)

// HandleChat runs a single conversation turn in-process (no daemon
// required): resolve agent path, assemble an Orchestrator, call the
// matching MessageHandler method directly, print the response, exit.
func HandleChat(args []string) error {
	// Parse arguments
	path := "."
	pathGiven := false
	userFlag := ""
	sessionKey := ""
	text := ""
	messageGiven := false
	debug := false
	inject := false
	role := ""
	model := ""
	provider := ""
	var deliver []string
	var tools []string

	rules := []flagRule{
		{names: []string{flagAgent, flagAgentS}, parse: func(args []string, i int) (int, error) {
			if i+1 >= len(args) {
				return 0, fmt.Errorf("%s requires a path or name", flagAgent)
			}
			path = args[i+1]
			pathGiven = true
			return 2, nil
		}},
		{names: []string{flagUser, flagUserS}, parse: func(args []string, i int) (int, error) {
			if i+1 >= len(args) {
				return 0, fmt.Errorf("%s requires an id", flagUser)
			}
			userFlag = args[i+1]
			return 2, nil
		}},
		{names: []string{flagSession, flagSessionS}, parse: func(args []string, i int) (int, error) {
			if i+1 >= len(args) {
				return 0, fmt.Errorf("%s requires an id", flagSession)
			}
			sessionKey = args[i+1]
			return 2, nil
		}},
		{names: []string{flagDeliver}, parse: func(args []string, i int) (int, error) {
			if i+1 >= len(args) {
				return 0, fmt.Errorf("%s requires a comma-separated list", flagDeliver)
			}
			deliver = parseCommaSeparated(args[i+1])
			return 2, nil
		}},
		{names: []string{flagInject}, parse: func(args []string, i int) (int, error) {
			inject = true
			consumed, resolvedRole := parseInjectArg(args, i)
			role = resolvedRole
			return consumed, nil
		}},
		{names: []string{flagTools}, parse: func(args []string, i int) (int, error) {
			if i+1 >= len(args) {
				return 0, fmt.Errorf("%s requires a comma-separated list", flagTools)
			}
			tools = parseCommaSeparated(args[i+1])
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
		{names: []string{flagDebug}, parse: func(args []string, i int) (int, error) {
			debug = true
			return 1, nil
		}},
		{names: []string{flagModel}, parse: func(args []string, i int) (int, error) {
			if i+1 >= len(args) {
				return 0, fmt.Errorf("%s requires a model name", flagModel)
			}
			model = args[i+1]
			return 2, nil
		}},
		{names: []string{flagProvider}, parse: func(args []string, i int) (int, error) {
			if i+1 >= len(args) {
				return 0, fmt.Errorf("%s requires a provider name", flagProvider)
			}
			provider = args[i+1]
			return 2, nil
		}},
	}

	if err := parseFlags(args, rules, func(arg string) error {
		return fmt.Errorf("unexpected argument: %s", arg)
	}); err != nil {
		return err
	}

	// Apply env vars (flags/args take priority)
	if !pathGiven {
		if envPath := os.Getenv(envAgent); envPath != "" {
			path = envPath
		}
	}
	if userFlag == "" {
		userFlag = os.Getenv(envUser)
	}
	if sessionKey == "" {
		sessionKey = os.Getenv(envSession)
	}
	if model == "" {
		model = os.Getenv(envModel)
	}
	if provider == "" {
		provider = os.Getenv(envProvider)
	}

	text, err := message.Resolve(text, messageGiven)
	if err != nil {
		return err
	}

	// Resolve agent path
	absPath, err := registry.ResolveAgentPath(path)
	if err != nil {
		return err
	}

	_, lg, err := loadAgentAndLogger(absPath, debug)
	if err != nil {
		return err
	}

	// Only construct Senders for gateways actually referenced by --deliver -
	// a plain chat with no delivery should never fail because e.g. telegram
	// is misconfigured.
	assembled, err := assembleOrchestrator(absPath, nil, lg, false, debug, deliveryGateways(deliver))
	if err != nil {
		return err
	}
	assembled.Orchestrator.Overrides = config.Overrides{Model: model, Provider: provider}
	defer assembled.Orchestrator.Wait()

	currentUser, uerr := user.Current()
	username := "unknown"
	if uerr == nil {
		username = currentUser.Username
	}

	hasDeliveryOptions := len(deliver) > 0 || len(tools) > 0

	var response string
	var debugSessionFile string

	switch {
	case hasDeliveryOptions:
		opts := internal.MessageOptions{
			Gateway:     cli.GatewayName,
			ContactID:   username,
			DisplayName: username,
			Content:     text,
			UserID:      userFlag,
			SessionID:   sessionKey,
			Deliver:     deliver,
			Inject:      inject,
			Role:        role,
			Tools:       tools,
		}
		response, err = assembled.Orchestrator.HandleMessageWithOptions(opts)

	case userFlag != "" || sessionKey != "":
		resolved, rerr := session.ResolveExplicit(assembled.Store, absPath, userFlag, sessionKey, cli.GatewayName)
		if rerr != nil {
			return fmt.Errorf("failed to find session: %w", rerr)
		}
		response, err = assembled.Orchestrator.HandleExplicitMessage(resolved.UserID, resolved.SessionID, cli.GatewayName, text)
		if debug {
			debugSessionFile = filepath.Join(".data", "sessions", resolved.UserID, resolved.SessionID+".jsonl")
		}

	default:
		cmd, cmdErr := syscommands.Parse(text)
		if cmdErr == nil {
			response, err = cli.HandleCommand(assembled.Store, absPath, username, cmd)
		} else {
			response, err = assembled.Orchestrator.HandleMessage(cli.GatewayName, username, username, "", text)
		}
	}

	if err != nil {
		return fmt.Errorf("agent error: %w", err)
	}

	// Print response with ANSI formatting if stdout is a terminal
	// Auto-detects TTY: formats for interactive use, plain text for pipes/redirects
	fmt.Println(cli.FormatForCLI(response))

	if debug && debugSessionFile != "" {
		fmt.Printf("\n[debug] session: %s\n", debugSessionFile)
	}

	return nil
}

// parseCommaSeparated splits a comma-separated string into a slice
func parseCommaSeparated(s string) []string {
	if s == "" {
		return nil
	}

	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
