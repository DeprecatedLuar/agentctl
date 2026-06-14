package commands

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/DeprecatedLuar/agentctl/internal/registry"
)

const (
	// File and directory names
	chatDataDir  = ".data"
	socketFilename = "agent.sock"

	// Flags
	flagAgent    = "--agent"
	flagAgentS   = "-a"
	flagUser     = "--user"
	flagUserS    = "-u"
	flagSession  = "--session"
	flagSessionS = "-s"
	// flagDebug is defined in run.go and shared across commands

	// Environment variables
	envUser    = "AGENTCTL_USER"
	envSession = "AGENTCTL_SESSION"
)

type chatRequest struct {
	User    string `json:"user"`
	Session string `json:"session"`
	Message string `json:"message"`
	Debug   bool   `json:"debug"`
}

type chatResponse struct {
	Response    string `json:"response"`
	Error       string `json:"error,omitempty"`
	SessionFile string `json:"session_file,omitempty"` // Populated when Debug=true
}

func HandleChat(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: agentctl chat <message> [--agent name|path] [--user id] [--session id] [--debug]")
	}

	// Parse arguments
	path := "."
	user := ""
	sessionKey := ""
	message := ""
	debug := false

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

	if message == "" {
		return fmt.Errorf("message is required")
	}

	// Apply env vars (flags take priority)
	if user == "" {
		user = os.Getenv(envUser)
	}
	if sessionKey == "" {
		sessionKey = os.Getenv(envSession)
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

	fmt.Println(resp.Response)

	// Print debug info if requested
	if debug && resp.SessionFile != "" {
		fmt.Printf("\n[debug] session: %s\n", resp.SessionFile)
	}

	return nil
}
