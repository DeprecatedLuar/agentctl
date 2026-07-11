package commands

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/DeprecatedLuar/agentctl/internal/interfaces/cli"
	"github.com/DeprecatedLuar/agentctl/internal/message"
	"github.com/DeprecatedLuar/agentctl/internal/registry"
)

const (
	// File and directory names
	chatDataDir    = ".data"
	socketFilename = "agent.sock"

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
	// flagDebug is defined in run.go and shared across commands

	// Environment variables
	envAgent   = "AGENTCTL_AGENT"
	envUser    = "AGENTCTL_USER"
	envSession = "AGENTCTL_SESSION"
)

type chatRequest struct {
	User    string   `json:"user"`
	Session string   `json:"session"`
	Message string   `json:"message"`
	Debug   bool     `json:"debug"`
	Deliver []string `json:"deliver,omitempty"`
	Inject  bool     `json:"inject,omitempty"`
	Role    string   `json:"role,omitempty"`
	Note    string   `json:"note,omitempty"`
	Tools   []string `json:"tools,omitempty"`
	Raw     bool     `json:"raw,omitempty"`
}

type chatResponse struct {
	Response    string `json:"response"`
	Error       string `json:"error,omitempty"`
	SessionFile string `json:"session_file,omitempty"` // Populated when Debug=true
}

func HandleChat(args []string) error {
	// Parse arguments
	path := "."
	pathGiven := false
	user := ""
	sessionKey := ""
	text := ""
	messageGiven := false
	debug := false
	inject := false
	role := ""
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
			user = args[i+1]
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
	if user == "" {
		user = os.Getenv(envUser)
	}
	if sessionKey == "" {
		sessionKey = os.Getenv(envSession)
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
		Session: sessionKey,
		Message: text,
		Debug:   debug,
		Deliver: deliver,
		Inject:  inject,
		Role:    role,
		Tools:   tools,
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

	// Print response with ANSI formatting if stdout is a terminal
	// Auto-detects TTY: formats for interactive use, plain text for pipes/redirects
	fmt.Println(cli.FormatForCLI(resp.Response))

	// Print debug info if requested
	if debug && resp.SessionFile != "" {
		fmt.Printf("\n[debug] session: %s\n", resp.SessionFile)
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
