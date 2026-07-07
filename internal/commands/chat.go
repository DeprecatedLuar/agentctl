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
	flagTools    = "--tools"
	flagMessage  = "--message"
	flagMessageS = "-m"
	// flagDebug is defined in run.go and shared across commands

	// Environment variables
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
	user := ""
	sessionKey := ""
	text := ""
	messageGiven := false
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
		case flagMessage, flagMessageS:
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires text", flagMessage)
			}
			text = args[i+1]
			messageGiven = true
			i += 2
		case flagDebug:
			debug = true
			i++
		default:
			return fmt.Errorf("unexpected argument: %s", args[i])
		}
	}

	// Apply env vars (flags/args take priority)
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
