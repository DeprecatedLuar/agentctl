package commands

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/DeprecatedLuar/agentctl/internal/agent"
	"github.com/DeprecatedLuar/agentctl/internal/config"
	"github.com/DeprecatedLuar/agentctl/internal/hooks"
	"github.com/DeprecatedLuar/agentctl/internal/registry"
	"github.com/DeprecatedLuar/agentctl/internal/resolution"
	"github.com/DeprecatedLuar/agentctl/internal/session"
)

// splitAgentFromToolPath splits an absolute tool file path into the agent
// root (the directory containing the last "tools" path segment) and the
// tool name relative to that tools/ directory (extension stripped).
func splitAgentFromToolPath(toolPath string) (agentPath string, toolName string, err error) {
	clean := filepath.Clean(toolPath)
	parts := strings.Split(clean, string(filepath.Separator))

	toolsIdx := -1
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == toolsDir {
			toolsIdx = i
			break
		}
	}
	if toolsIdx <= 0 || toolsIdx == len(parts)-1 {
		return "", "", fmt.Errorf("tool path %q must be inside an agent's %s/ directory", toolPath, toolsDir)
	}

	agentPath = string(filepath.Separator) + filepath.Join(parts[:toolsIdx]...)
	toolName = filepath.Join(parts[toolsIdx+1:]...)
	toolName = strings.TrimSuffix(toolName, ".toml")

	return agentPath, toolName, nil
}

func HandleToolRun(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: agentctl toolrun <tool-name> [--param=value ...] [--agent path]")
	}

	// Parse arguments
	agentPath := "."
	toolName := ""
	params := make(map[string]interface{})

	i := 0
	for i < len(args) {
		arg := args[i]

		// Check for --agent/-a flag
		if arg == flagAgent || arg == flagAgentS {
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a path or name", flagAgent)
			}
			agentPath = args[i+1]
			i += 2
			continue
		}

		// Check for --param=value format
		if strings.HasPrefix(arg, "--") && strings.Contains(arg, "=") {
			parts := strings.SplitN(arg[2:], "=", 2)
			if len(parts) == 2 {
				params[parts[0]] = parts[1]
			}
			i++
			continue
		}

		// First non-flag argument is the tool name
		if toolName == "" {
			toolName = arg
			i++
			continue
		}

		// Unknown argument
		return fmt.Errorf("unknown argument: %s", arg)
	}

	if toolName == "" {
		return fmt.Errorf("tool name is required")
	}

	// A full path to a tool file carries its own agent root: it's always the
	// directory containing the "tools" segment. Derive agentPath/toolName
	// from it, overriding any --agent flag.
	if filepath.IsAbs(toolName) {
		derivedAgentPath, relToolName, err := splitAgentFromToolPath(toolName)
		if err != nil {
			return err
		}
		agentPath = derivedAgentPath
		toolName = relToolName
	}

	// Resolve agent path
	absPath, err := registry.ResolveAgentPath(agentPath)
	if err != nil {
		return err
	}

	// Load agent config to get environment
	agentCfg, agentIssues := config.LoadAgent(absPath)
	if agentCfg == nil {
		for _, issue := range agentIssues {
			if issue.Type == config.IssueError {
				return fmt.Errorf("agent config error: %s", issue.Message)
			}
		}
		return fmt.Errorf("failed to load agent config")
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Resolve CLI user context (same as CLI interface does)
	systemUser := os.Getenv("USER")
	if systemUser == "" {
		systemUser = "unknown"
	}
	platformID := systemUser
	userID, err := session.ResolveUser(absPath, "cli", platformID)
	if err != nil {
		// Non-fatal: continue with system username as fallback
		userID = systemUser
	}

	// Build context with CLI user (sessionID empty for one-off execution)
	ctx := resolution.NewContext(
		absPath,
		userID,     // resolved user ID from contacts
		systemUser, // display name
		"",         // sessionID (no session for one-off tool execution)
		"cli",      // interface
		agentCfg.Agent.Model,
		agentCfg.Agent.Provider,
		agentCfg.Environment,
	)

	// Execute prerun hook before tool load, same as the message-handling pipeline
	if err := hooks.ExecutePrerun(absPath, agentCfg.Environment, resolution.SystemEnv(ctx), logger); err != nil {
		logger.Error("prerun hook error", "error", err)
	}

	// Load the specific tool
	tools, issues := config.LoadTools(absPath, []string{toolName})

	// Print validation issues if any
	for _, issue := range issues {
		if issue.Type == config.IssueError {
			return fmt.Errorf("tool load error: %s", issue.Message)
		}
		if issue.Type == config.IssueWarning {
			fmt.Fprintf(os.Stderr, "warning: %s\n", issue.Message)
		}
	}

	if len(tools) == 0 {
		return fmt.Errorf("tool '%s' not found in %s/tools/", toolName, absPath)
	}

	tool := tools[0]

	result := agent.ExecuteTool(&tool, params, absPath, ctx, logger, false, true)

	// Print result
	fmt.Print(result.Output)

	return nil
}
