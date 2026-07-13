package resolution

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Context holds all runtime information for template processing.
// Used for both directive processing and variable substitution.
type Context struct {
	AgentPath string            // Absolute path to agent folder (fixed root, used for .env/config lookups)
	BaseDir   string            // Directory that relative {{file:}}/{{exec:}} paths in the content currently being processed resolve against
	AgentName string            // Base name of agent folder
	UserID    string            // User identity ID (e.g., "alice" or "cli:luar")
	Username  string            // Display name for user
	SessionID string            // Current session ID
	Gateway   string            // Gateway name ("cli", "telegram", etc.)
	Model     string            // Model name (e.g., "gpt-4o")
	Provider  string            // Provider name (e.g., "openai")
	Timestamp time.Time         // Current timestamp
	UserVars  map[string]string // User-defined variables (future use)
	ConfigEnv map[string]string // Config environment from [environment] section
}

// NewContext creates a runtime Context with all fields populated.
// Used during message processing when full runtime information is available.
// Empty strings are valid for optional fields (username can be empty if not available).
func NewContext(agentPath, userID, username, sessionID, gateway, model, provider string, configEnv map[string]string) Context {
	return Context{
		AgentPath: agentPath,
		BaseDir:   agentPath,
		AgentName: filepath.Base(agentPath),
		UserID:    userID,
		Username:  username,
		SessionID: sessionID,
		Gateway:   gateway,
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
		BaseDir:   agentPath,
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
	vars["$agentpath"] = ctx.AgentPath
	vars["$user"] = ctx.UserID
	vars["$username"] = ctx.Username
	vars["$session"] = ctx.SessionID
	vars["$gateway"] = ctx.Gateway
	// $interface is a deprecated alias for $gateway, kept for backward
	// compatibility with existing prompts/tools - drop once callers migrate.
	vars["$interface"] = ctx.Gateway

	// Time formatting
	if !ctx.Timestamp.IsZero() {
		vars["$timestamp"] = ctx.Timestamp.Format(time.RFC3339)
		vars["$date"] = ctx.Timestamp.Format("2006-01-02")
		vars["$now"] = formatNow(ctx.Timestamp)
	} else {
		vars["$timestamp"] = ""
		vars["$date"] = ""
		vars["$now"] = ""
	}

	vars["$model"] = ctx.Model
	vars["$provider"] = ctx.Provider

	return vars
}

// SystemEnv returns the same values as {{$var}} template substitution
// (buildSystemVariables), as AGENTCTL_-prefixed env vars, for injection
// into subprocess environments (prerun scripts, tool shell commands). This
// naturally produces both AGENTCTL_GATEWAY and the deprecated
// AGENTCTL_INTERFACE alias, since both keys exist in buildSystemVariables.
func SystemEnv(ctx Context) map[string]string {
	sysVars := buildSystemVariables(ctx)
	env := make(map[string]string, len(sysVars))
	for name, value := range sysVars {
		envKey := "AGENTCTL_" + strings.ToUpper(strings.TrimPrefix(name, "$"))
		env[envKey] = value
	}
	return env
}

// formatNow renders a human-readable timestamp like
// "Tuesday, July 2, 2026, 2:23 PM (GMT-3)".
func formatNow(t time.Time) string {
	_, offsetSeconds := t.Zone()
	offsetHours := offsetSeconds / 3600
	return fmt.Sprintf("%s (GMT%+d)", t.Format("Monday, January 2, 2006, 3:04 PM"), offsetHours)
}
