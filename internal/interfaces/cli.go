package interfaces

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/DeprecatedLuar/agentctl/internal/agent"
)

const (
	// Directory and file names
	cliDataDir     = ".data"
	cliSocketFile  = "agent.sock"
	dirPermissions = 0755

	// Interface name
	interfaceNameCLI = "cli"

	// Session defaults
	defaultSessionKey = "default"
)

// CLIInterface implements the Unix socket interface for local CLI access
type CLIInterface struct {
	socketPath string
}

// CLIRequest represents a request from the CLI client
type CLIRequest struct {
	Session string `json:"session"`
	Message string `json:"message"`
}

// CLIResponse represents a response to the CLI client
type CLIResponse struct {
	Response string `json:"response"`
	Error    string `json:"error,omitempty"`
}

// NewCLI creates a new CLI interface
func NewCLI(agentFolder string) *CLIInterface {
	socketPath := filepath.Join(agentFolder, cliDataDir, cliSocketFile)
	return &CLIInterface{socketPath: socketPath}
}

// Start begins listening on the Unix socket
func (c *CLIInterface) Start(ctx context.Context, runner *Runner) error {
	// Ensure .data directory exists
	dataDir := filepath.Dir(c.socketPath)
	if err := os.MkdirAll(dataDir, dirPermissions); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Remove existing socket file
	_ = os.Remove(c.socketPath)

	// Create Unix socket listener
	listener, err := net.Listen("unix", c.socketPath)
	if err != nil {
		return fmt.Errorf("failed to create socket: %w", err)
	}
	defer listener.Close()

	fmt.Printf("CLI interface listening on %s\n", c.socketPath)

	// Accept connections in a loop
	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				fmt.Printf("Accept error: %v\n", err)
				continue
			}
		}

		go c.handleConnection(conn, runner)
	}
}

func (c *CLIInterface) handleConnection(conn net.Conn, runner *Runner) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	encoder := json.NewEncoder(conn)

	for scanner.Scan() {
		var req CLIRequest
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			encoder.Encode(CLIResponse{Error: fmt.Sprintf("invalid request: %v", err)})
			continue
		}

		// Default session key to "default"
		sessionKey := req.Session
		if sessionKey == "" {
			sessionKey = defaultSessionKey
		}

		// Log message received
		if runner.Logger != nil {
			msg := fmt.Sprintf("message received %s:%s", sessionKey, interfaceNameCLI)
			if runner.Verbose {
				runner.Logger.Info(msg, "content", req.Message)
			} else {
				runner.Logger.Info(msg)
			}
		}

		// Run agent
		response, err := runner.Run(agent.Input{
			SessionKey: sessionKey,
			Content:    req.Message,
		})

		if err != nil {
			if runner.Logger != nil {
				runner.Logger.Error(fmt.Sprintf("agent error %s:%s", sessionKey, interfaceNameCLI), "error", err)
			}
			encoder.Encode(CLIResponse{Error: err.Error()})
		} else {
			// Log response sent
			if runner.Logger != nil {
				msg := fmt.Sprintf("response sent %s:%s", sessionKey, interfaceNameCLI)
				if runner.Verbose {
					runner.Logger.Info(msg, "content", response)
				} else {
					runner.Logger.Info(msg)
				}
			}
			encoder.Encode(CLIResponse{Response: response})
		}
	}
}
