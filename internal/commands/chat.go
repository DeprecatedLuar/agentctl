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
	flagTools    = "--tools"
	// flagDebug is defined in run.go and shared across commands

	// Environment variables
	envUser    = "AGENTCTL_USER"
	envSession = "AGENTCTL_SESSION"
	envMessage = "AGENTCTL_MESSAGE"
)

type chatRequest struct {
	User    string   `json:"user"`
	Session string   `json:"session"`
	Message string   `json:"message"`
	Debug   bool     `json:"debug"`
	Deliver []string `json:"deliver,omitempty"`
	Inject  bool     `json:"inject,omitempty"`
	Tools   []string `json:"tools,omitempty"`
}

type chatResponse struct {
	Response    string `json:"response"`
	Error       string `json:"error,omitempty"`
	SessionFile string `json:"session_file,omitempty"` // Populated when Debug=true
}

func HandleChat(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: agentctl chat <message> [--agent name|path] [--user id] [--session id] [--deliver list] [--inject] [--tools list] [--debug]")
	}

	// Parse arguments
	path := "."
	user := ""
	sessionKey := ""
	message := ""
	debug := false
	inject := false
	var deliver []string
	var tools []string

	i := 0
	for i < len(args) {
		switch args[i] {
		case flagAgent, flagAgentS:
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a path or name", flagAgent)
			}
			path = args[i+1]
			i += 2
		case flagUser, flagUserS:
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires an id", flagUser)
			}
			user = args[i+1]
			i += 2
		case flagSession, flagSessionS:
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires an id", flagSession)
			}
			sessionKey = args[i+1]
			i += 2
		case flagDeliver:
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a comma-separated list", flagDeliver)
			}
			deliver = parseCommaSeparated(args[i+1])
			i += 2
		case flagInject:
			inject = true
			i++
		case flagTools:
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a comma-separated list", flagTools)
			}
			tools = parseCommaSeparated(args[i+1])
			i += 2
		case flagDebug:
			debug = true
			i++
		default:
			// First non-flag argument is the message
			if message == "" {
				message = args[i]
			}
			i++
		}
	}

	// Apply env vars (flags/args take priority)
	if user == "" {
		user = os.Getenv(envUser)
	}
	if sessionKey == "" {
		sessionKey = os.Getenv(envSession)
	}
	if message == "" {
		message = os.Getenv(envMessage)
	}

	if message == "" {
		return fmt.Errorf("message is required")
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
		Message: message,
		Debug:   debug,
		Deliver: deliver,
		Inject:  inject,
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
