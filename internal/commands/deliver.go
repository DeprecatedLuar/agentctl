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

// HandleDeliver sends literal text straight to one or more channels via the
// running agent daemon, bypassing the LLM entirely (unlike chat).
func HandleDeliver(args []string) error {
	// Parse arguments
	path := "."
	user := ""
	message := ""
	inject := false
	var deliver []string

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
		case flagDeliver:
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a comma-separated list", flagDeliver)
			}
			deliver = parseCommaSeparated(args[i+1])
			i += 2
		case flagInject:
			inject = true
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
	if message == "" {
		message = os.Getenv(envMessage)
	}

	if message == "" {
		return fmt.Errorf("message is required")
	}
	if len(deliver) == 0 {
		return fmt.Errorf("usage: agentctl deliver <message> --deliver <list> [--inject] [--agent name|path] [--user id]")
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
		Message: message,
		Deliver: deliver,
		Inject:  inject,
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
