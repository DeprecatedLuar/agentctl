package cli

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
	"github.com/DeprecatedLuar/agentctl/internal/logger"
	"github.com/DeprecatedLuar/agentctl/internal/session"
	"github.com/DeprecatedLuar/agentctl/internal/syscommands"
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
	User    string   `json:"user"`
	Session string   `json:"session"`
	Message string   `json:"message"`
	Debug   bool     `json:"debug"`
	Deliver []string `json:"deliver,omitempty"`
	Inject  bool     `json:"inject,omitempty"`
	Tools   []string `json:"tools,omitempty"`
	Raw     bool     `json:"raw,omitempty"`
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
	if err := os.Remove(c.socketPath); err != nil && !os.IsNotExist(err) {
		if c.logger != nil {
			c.logger.Warn("failed to remove existing socket file", "socket", c.socketPath, "error", err)
		}
	}

	// Create Unix socket listener
	listener, err := net.Listen("unix", c.socketPath)
	if err != nil {
		return fmt.Errorf("failed to create socket: %w", err)
	}
	defer listener.Close()

	if c.logger != nil {
		c.logger.Info("cli listening", "socket", c.socketPath)
	}

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

// encode writes a CLIResponse to the client and logs any encode error
// instead of silently discarding it.
func (c *CLIInterface) encode(encoder *json.Encoder, resp CLIResponse) {
	if err := encoder.Encode(resp); err != nil && c.logger != nil {
		c.logger.Error("failed to encode CLI response", "error", err)
	}
}

func (c *CLIInterface) handleConnection(conn net.Conn) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	encoder := json.NewEncoder(conn)

	for scanner.Scan() {
		var req CLIRequest
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			c.encode(encoder, CLIResponse{Error: fmt.Sprintf("invalid request: %v", err)})
			continue
		}

		// Check if we need to use HandleMessageWithOptions
		hasDeliveryOptions := len(req.Deliver) > 0 || len(req.Tools) > 0

		if hasDeliveryOptions {
			c.handleWithOptions(encoder, req)
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
		username := "" // CLI doesn't have @handles

		// Check if message is a command
		cmd, cmdErr := syscommands.Parse(req.Message)
		if cmdErr == nil {
			// Handle command in CLI layer
			response, err := c.handleCommand(cmd)
			if err != nil {
				c.encode(encoder, CLIResponse{Error: err.Error()})
			} else {
				c.encode(encoder, CLIResponse{Response: response})
			}
			continue
		}

		// Log message received
		if c.logger != nil {
			msg := fmt.Sprintf("message received %s:%s", contactID, interfaceNameCLI)
			if c.verbose {
				c.logger.Info(msg, "kind", logger.KindRecv, "content", req.Message)
			} else {
				c.logger.Info(msg, "kind", logger.KindRecv)
			}
		}

		// Handle message via service layer (pure I/O adapter)
		response, runErr := c.handler.HandleMessage(interfaceNameCLI, contactID, displayName, username, req.Message)

		if runErr != nil {
			if c.logger != nil {
				c.logger.Error("agent error", "contact", contactID, "interface", interfaceNameCLI, "error", runErr)
			}
			c.encode(encoder, CLIResponse{Error: runErr.Error()})
		} else {
			// Log response sent
			if c.logger != nil {
				msg := fmt.Sprintf("response sent %s:%s", contactID, interfaceNameCLI)
				if c.verbose {
					c.logger.Info(msg, "kind", logger.KindSent, "content", response)
				} else {
					c.logger.Info(msg, "kind", logger.KindSent)
				}
			}

			// Build response (no debug info in normal flow since we don't know resolved session)
			resp := CLIResponse{Response: response}
			c.encode(encoder, resp)
		}
	}
}

// handleExplicitResolution handles the special case of --user/--session flags
// This bypasses normal contact resolution and directly specifies user/session
func (c *CLIInterface) handleExplicitResolution(encoder *json.Encoder, req CLIRequest) {
	// Resolve explicit user/session
	resolved, err := session.ResolveExplicit(c.store, c.agentFolder, req.User, req.Session, interfaceNameCLI)
	if err != nil {
		c.encode(encoder, CLIResponse{Error: fmt.Sprintf("explicit resolution failed: %v", err)})
		return
	}

	// Log message received
	if c.logger != nil {
		msg := fmt.Sprintf("message received %s:%s (explicit)", resolved.UserID, interfaceNameCLI)
		if c.verbose {
			c.logger.Info(msg, "kind", logger.KindRecv, "content", req.Message)
		} else {
			c.logger.Info(msg, "kind", logger.KindRecv)
		}
	}

	// Use explicit message handler (bypasses contact resolution)
	response, runErr := c.handler.HandleExplicitMessage(resolved.UserID, resolved.SessionID, interfaceNameCLI, req.Message)

	if runErr != nil {
		if c.logger != nil {
			c.logger.Error("agent error", "user", resolved.UserID, "interface", interfaceNameCLI, "error", runErr)
		}
		c.encode(encoder, CLIResponse{Error: runErr.Error()})
	} else {
		// Log response sent
		if c.logger != nil {
			msg := fmt.Sprintf("response sent %s:%s (explicit)", resolved.UserID, interfaceNameCLI)
			if c.verbose {
				c.logger.Info(msg, "kind", logger.KindSent, "content", response)
			} else {
				c.logger.Info(msg, "kind", logger.KindSent)
			}
		}

		// Build response with debug info
		resp := CLIResponse{Response: response}
		if req.Debug {
			sessionFile := filepath.Join(".data", "sessions", resolved.UserID, resolved.SessionID+".jsonl")
			resp.SessionFile = sessionFile
		}
		c.encode(encoder, resp)
	}
}

// handleWithOptions handles requests with delivery options (--deliver, --inject, --tools)
func (c *CLIInterface) handleWithOptions(encoder *json.Encoder, req CLIRequest) {
	// Resolve user ID for channel resolution (need identity ID, not raw username)
	var userID string
	if req.User != "" {
		userID = req.User
	} else {
		// Resolve username to identity ID
		resolvedUserID, err := c.store.ResolveUser(interfaceNameCLI, c.username)
		if err != nil {
			c.encode(encoder, CLIResponse{Error: fmt.Sprintf("failed to resolve user: %v", err)})
			return
		}
		userID = resolvedUserID
	}

	// Build MessageOptions
	opts := internal.MessageOptions{
		Interface:   interfaceNameCLI,
		ContactID:   c.username,
		DisplayName: c.username,
		Username:    "", // CLI doesn't have @handles
		Content:     req.Message,
		UserID:      req.User,    // Empty unless --user specified
		SessionID:   req.Session, // Empty unless --session specified
		Deliver:     req.Deliver,
		Inject:      req.Inject,
		Tools:       req.Tools,
		Raw:         req.Raw,
	}

	// Log message received
	if c.logger != nil {
		msg := fmt.Sprintf("message received %s:%s (with options)", userID, interfaceNameCLI)
		if c.verbose {
			c.logger.Info(msg, "kind", logger.KindRecv, "content", req.Message)
		} else {
			c.logger.Info(msg, "kind", logger.KindRecv)
		}
	}

	// Handle message with options
	response, runErr := c.handler.HandleMessageWithOptions(opts)

	if runErr != nil {
		if c.logger != nil {
			c.logger.Error("agent error", "user", userID, "interface", interfaceNameCLI, "error", runErr)
		}
		c.encode(encoder, CLIResponse{Error: runErr.Error()})
	} else {
		// Log response sent
		if c.logger != nil {
			msg := fmt.Sprintf("response sent %s:%s (with options)", userID, interfaceNameCLI)
			if c.verbose {
				c.logger.Info(msg, "kind", logger.KindSent, "content", response)
			} else {
				c.logger.Info(msg, "kind", logger.KindSent)
			}
		}

		// Build response
		resp := CLIResponse{Response: response}
		c.encode(encoder, resp)
	}
}

// InterfaceName returns the interface identifier for Sender interface
func (c *CLIInterface) InterfaceName() string {
	return interfaceNameCLI
}

// Send delivers a message to the specified CLI user (Sender interface)
// Not yet implemented - requires interactive mode support
func (c *CLIInterface) Send(platformID, content string) error {
	return fmt.Errorf("CLI outbound delivery requires interactive mode, not yet implemented")
}
