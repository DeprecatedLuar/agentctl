package commands

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/DeprecatedLuar/agentctl/internal/agent"
	"github.com/DeprecatedLuar/agentctl/internal/config"
	"github.com/DeprecatedLuar/agentctl/internal/registry"
)

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

	// Execute the tool (with debug enabled to show command)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	result := agent.ExecuteTool(&tool, params, absPath, agentCfg.Environment, logger, false, true)

	// Print result
	fmt.Print(result)

	return nil
}
