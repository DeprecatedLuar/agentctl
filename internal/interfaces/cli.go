package interfaces

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"

	"github.com/DeprecatedLuar/agentctl/internal/agent"
	"github.com/DeprecatedLuar/agentctl/internal/session"
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
	socketPath  string
	agentFolder string
	username    string
}

// CLIRequest represents a request from the CLI client
type CLIRequest struct {
	User    string `json:"user"`
	Session string `json:"session"`
	Message string `json:"message"`
	Debug   bool   `json:"debug"`
}

// CLIResponse represents a response to the CLI client
type CLIResponse struct {
	Response    string `json:"response"`
	Error       string `json:"error,omitempty"`
	SessionFile string `json:"session_file,omitempty"` // Populated when Debug=true
}

// NewCLI creates a new CLI interface
func NewCLI(agentFolder string) *CLIInterface {
	socketPath := filepath.Join(agentFolder, cliDataDir, cliSocketFile)

	// Get current username
	currentUser, err := user.Current()
	username := "unknown"
	if err == nil {
		username = currentUser.Username
	}

	return &CLIInterface{
		socketPath:  socketPath,
		agentFolder: agentFolder,
		username:    username,
	}
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

		// Resolve session
		var resolved session.ResolvedSession
		var err error

		// If explicit user/session provided, use ResolveExplicit
		if req.User != "" || req.Session != "" {
			resolved, err = session.ResolveExplicit(c.agentFolder, req.User, req.Session, interfaceNameCLI)
		} else {
			// Otherwise use automatic resolution
			resolved, err = session.Resolve(c.agentFolder, interfaceNameCLI, c.username, c.username)
		}

		if err != nil {
			encoder.Encode(CLIResponse{Error: fmt.Sprintf("session resolution failed: %v", err)})
			continue
		}

		// Log message received
		if runner.Logger != nil {
			msg := fmt.Sprintf("message received %s:%s", resolved.UserID, interfaceNameCLI)
			if runner.Verbose {
				runner.Logger.Info(msg, "content", req.Message)
			} else {
				runner.Logger.Info(msg)
			}
		}

		// Run agent
		response, runErr := runner.Run(agent.Input{
			UserID:    resolved.UserID,
			SessionID: resolved.SessionID,
			Interface: interfaceNameCLI,
			Content:   req.Message,
		})

		if runErr != nil {
			if runner.Logger != nil {
				runner.Logger.Error(fmt.Sprintf("agent error %s:%s", resolved.UserID, interfaceNameCLI), "error", runErr)
			}
			encoder.Encode(CLIResponse{Error: runErr.Error()})
		} else {
			// Log response sent
			if runner.Logger != nil {
				msg := fmt.Sprintf("response sent %s:%s", resolved.UserID, interfaceNameCLI)
				if runner.Verbose {
					runner.Logger.Info(msg, "content", response)
				} else {
					runner.Logger.Info(msg)
				}
			}

			// Build response with optional debug info
			resp := CLIResponse{Response: response}
			if req.Debug {
				sessionFile := filepath.Join(".data", "sessions", resolved.UserID, resolved.SessionID+".jsonl")
				resp.SessionFile = sessionFile
			}
			encoder.Encode(resp)
		}
	}
}
