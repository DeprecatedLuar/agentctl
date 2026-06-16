package interfaces

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/user"
	"path/filepath"

	"github.com/DeprecatedLuar/agentctl/internal"
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
// Note: Unlike other interfaces, CLI retains minimal session access for explicit
// resolution (--user/--session flags). This is the only exception to pure I/O adapter rule.
type CLIInterface struct {
	socketPath  string
	agentFolder string
	username    string
	handler     internal.MessageHandler
	store       session.SessionStore // Only for explicit resolution edge case
	logger      *slog.Logger
	verbose     bool
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
func NewCLI(agentFolder string, handler internal.MessageHandler, store session.SessionStore, logger *slog.Logger, verbose bool) *CLIInterface {
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
		handler:     handler,
		store:       store,
		logger:      logger,
		verbose:     verbose,
	}
}

// Start begins listening on the Unix socket
func (c *CLIInterface) Start(ctx context.Context) error {
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

		go c.handleConnection(conn)
	}
}

func (c *CLIInterface) handleConnection(conn net.Conn) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	encoder := json.NewEncoder(conn)

	for scanner.Scan() {
		var req CLIRequest
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			encoder.Encode(CLIResponse{Error: fmt.Sprintf("invalid request: %v", err)})
			continue
		}

		// Handle explicit resolution (--user/--session flags)
		// This is the ONLY exception to pure I/O adapter rule - CLI has a unique
		// explicit resolution feature that bypasses normal contact resolution
		if req.User != "" || req.Session != "" {
			c.handleExplicitResolution(encoder, req)
			continue
		}

		// Normal flow: Use username as contact ID
		contactID := c.username
		displayName := c.username

		// Log message received
		if c.logger != nil {
			msg := fmt.Sprintf("message received %s:%s", contactID, interfaceNameCLI)
			if c.verbose {
				c.logger.Info(msg, "content", req.Message)
			} else {
				c.logger.Info(msg)
			}
		}

		// Handle message via service layer (pure I/O adapter)
		response, runErr := c.handler.HandleMessage(interfaceNameCLI, contactID, displayName, req.Message)

		if runErr != nil {
			if c.logger != nil {
				c.logger.Error("agent error", "contact", contactID, "interface", interfaceNameCLI, "error", runErr)
			}
			encoder.Encode(CLIResponse{Error: runErr.Error()})
		} else {
			// Log response sent
			if c.logger != nil {
				msg := fmt.Sprintf("response sent %s:%s", contactID, interfaceNameCLI)
				if c.verbose {
					c.logger.Info(msg, "content", response)
				} else {
					c.logger.Info(msg)
				}
			}

			// Build response (no debug info in normal flow since we don't know resolved session)
			resp := CLIResponse{Response: response}
			encoder.Encode(resp)
		}
	}
}

// handleExplicitResolution handles the special case of --user/--session flags
// This bypasses normal contact resolution and directly specifies user/session
func (c *CLIInterface) handleExplicitResolution(encoder *json.Encoder, req CLIRequest) {
	// Resolve explicit user/session
	resolved, err := session.ResolveExplicit(c.store, c.agentFolder, req.User, req.Session, interfaceNameCLI)
	if err != nil {
		encoder.Encode(CLIResponse{Error: fmt.Sprintf("explicit resolution failed: %v", err)})
		return
	}

	// Log message received
	if c.logger != nil {
		msg := fmt.Sprintf("message received %s:%s (explicit)", resolved.UserID, interfaceNameCLI)
		if c.verbose {
			c.logger.Info(msg, "content", req.Message)
		} else {
			c.logger.Info(msg)
		}
	}

	// Use explicit message handler (bypasses contact resolution)
	response, runErr := c.handler.HandleExplicitMessage(resolved.UserID, resolved.SessionID, interfaceNameCLI, req.Message)

	if runErr != nil {
		if c.logger != nil {
			c.logger.Error("agent error", "user", resolved.UserID, "interface", interfaceNameCLI, "error", runErr)
		}
		encoder.Encode(CLIResponse{Error: runErr.Error()})
	} else {
		// Log response sent
		if c.logger != nil {
			msg := fmt.Sprintf("response sent %s:%s (explicit)", resolved.UserID, interfaceNameCLI)
			if c.verbose {
				c.logger.Info(msg, "content", response)
			} else {
				c.logger.Info(msg)
			}
		}

		// Build response with debug info
		resp := CLIResponse{Response: response}
		if req.Debug {
			sessionFile := filepath.Join(".data", "sessions", resolved.UserID, resolved.SessionID+".jsonl")
			resp.SessionFile = sessionFile
		}
		encoder.Encode(resp)
	}
}
