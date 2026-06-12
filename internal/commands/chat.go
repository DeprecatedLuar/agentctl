package commands

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/DeprecatedLuar/agentctl/internal/registry"
)

const (
	// File and directory names (shared with run.go)
	chatAgentConfigFile = "agent.toml"
	chatDataDir         = ".data"
	socketFilename      = "agent.sock"

	// Flags
	flagAgent   = "--agent"
	flagAgentS  = "-a"
	flagSession = "--session"
	flagSessionS = "-s"
)

type chatRequest struct {
	Session string `json:"session"`
	Message string `json:"message"`
}

type chatResponse struct {
	Response string `json:"response"`
	Error    string `json:"error,omitempty"`
}

func HandleChat(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: agentctl chat <message> [--agent name|path] [--session key]")
	}

	// Parse arguments
	path := "."
	sessionKey := ""
	message := ""

	i := 0
	for i < len(args) {
		switch args[i] {
		case flagAgent, flagAgentS:
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a path or name", flagAgent)
			}
			path = args[i+1]
			i += 2
		case flagSession, flagSessionS:
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a key", flagSession)
			}
			sessionKey = args[i+1]
			i += 2
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

	// Resolve agent path: treat as path if contains / or starts with ., otherwise lookup by name
	var absPath string
	var err error

	if strings.Contains(path, "/") || strings.HasPrefix(path, ".") {
		// Treat as path
		absPath, err = filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
		if _, err := os.Stat(absPath); err != nil {
			return fmt.Errorf("agent path does not exist: %s", absPath)
		}
	} else {
		// Treat as name, resolve from registry
		absPath, err = registry.Resolve(path)
		if err != nil {
			return err
		}
	}

	// Validate agent folder
	agentTomlPath := filepath.Join(absPath, chatAgentConfigFile)
	if _, err := os.Stat(agentTomlPath); err != nil {
		return fmt.Errorf("not an agent folder (%s not found)", chatAgentConfigFile)
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
		Session: sessionKey,
		Message: message,
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
	return nil
}
