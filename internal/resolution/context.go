package resolution

import (
	"path/filepath"
	"time"
)

// Context holds all runtime information for template processing.
// Used for both directive processing and variable substitution.
type Context struct {
	AgentPath string            // Absolute path to agent folder
	AgentName string            // Base name of agent folder
	UserID    string            // User identity ID (e.g., "alice" or "cli:luar")
	Username  string            // Display name for user
	SessionID string            // Current session ID
	Interface string            // Interface name ("cli", "telegram", etc.)
	Model     string            // Model name (e.g., "gpt-4o")
	Provider  string            // Provider name (e.g., "openai")
	Timestamp time.Time         // Current timestamp
	UserVars  map[string]string // User-defined variables (future use)
	ConfigEnv map[string]string // Config environment from [environment] section
}

// NewContext creates a runtime Context with all fields populated.
// Used during message processing when full runtime information is available.
// Empty strings are valid for optional fields (username can be empty if not available).
func NewContext(agentPath, userID, username, sessionID, iface, model, provider string, configEnv map[string]string) Context {
	return Context{
		AgentPath: agentPath,
		AgentName: filepath.Base(agentPath),
		UserID:    userID,
		Username:  username,
		SessionID: sessionID,
		Interface: iface,
		Model:     model,
		Provider:  provider,
		Timestamp: time.Now(),
		UserVars:  nil, // Future use
		ConfigEnv: configEnv,
	}
}

// NewValidationContext creates a minimal Context for startup validation.
// Only AgentPath and AgentName are required - all other fields remain zero values.
// Validation only checks syntax, not actual runtime values.
func NewValidationContext(agentPath string) Context {
	return Context{
		AgentPath: agentPath,
		AgentName: filepath.Base(agentPath),
		// All other fields: zero values (empty strings, zero time)
	}
}

// buildSystemVariables creates the system variable map from context.
// Always populates all keys, even if values are empty (validation mode).
// System variables use $ prefix: {{$agent}}, {{$user}}, etc.
func buildSystemVariables(ctx Context) map[string]string {
	vars := make(map[string]string)

	// Always set keys, even if values are empty (during validation)
	vars["$agent"] = ctx.AgentName
	vars["$user"] = ctx.UserID
	vars["$username"] = ctx.Username
	vars["$session"] = ctx.SessionID
	vars["$interface"] = ctx.Interface

	// Time formatting
	if !ctx.Timestamp.IsZero() {
		vars["$timestamp"] = ctx.Timestamp.Format(time.RFC3339)
		vars["$date"] = ctx.Timestamp.Format("2006-01-02")
	} else {
		vars["$timestamp"] = ""
		vars["$date"] = ""
	}

	vars["$model"] = ctx.Model
	vars["$provider"] = ctx.Provider

	return vars
}
