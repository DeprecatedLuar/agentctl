package commands

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/DeprecatedLuar/agentctl/internal/message"
	"github.com/DeprecatedLuar/agentctl/internal/registry"
)

// HandleDeliver sends literal text straight to one or more channels via the
// running agent daemon, bypassing the LLM entirely (unlike chat).
func HandleDeliver(args []string) error {
	// Parse arguments
	path := "."
	user := ""
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
			user = args[i+1]
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
	if user == "" {
		user = os.Getenv(envUser)
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

	// Connect to socket
	socketPath := filepath.Join(absPath, chatDataDir, socketFilename)
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return fmt.Errorf("failed to connect to agent (is it running?): %w", err)
	}
	defer conn.Close()

	// Send request
	req := chatRequest{
		User:    user,
		Message: text,
		Deliver: deliver,
		Inject:  inject,
		Role:    role,
		Note:    note,
		Raw:     true,
	}
	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(req); err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	// Read response
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return fmt.Errorf("no response from agent")
	}

	var resp chatResponse
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		return fmt.Errorf("invalid response: %w", err)
	}

	if resp.Error != "" {
		return fmt.Errorf("agent error: %s", resp.Error)
	}

	fmt.Printf("Delivered to: %v\n", deliver)
	return nil
}
