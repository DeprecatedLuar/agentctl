package agent

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/DeprecatedLuar/agentctl/internal/config"
	debugpkg "github.com/DeprecatedLuar/agentctl/internal/debug"
	"github.com/DeprecatedLuar/agentctl/internal/directives"
	"github.com/DeprecatedLuar/agentctl/internal/shell"
)

const (
	// Placeholder format
	placeholderFormat = "{{%s}}"

	// Error format
	exitCodeFormat = "exit %d: %s"
)

// ExecuteTool runs a tool with the given arguments
func ExecuteTool(tool *config.ToolConfig, args map[string]interface{}, agentFolder string, logger *slog.Logger, verbose bool, debug bool) string {
	// Build substitution map: Process return overrides with directive support
	substitutions := make(map[string]string)

	// First, add AI-provided arguments
	for key, value := range args {
		substitutions[key] = fmt.Sprintf("%v", value)
	}

	// Then, process return overrides (takes precedence over AI args)
	for paramName, param := range tool.Parameters {
		// Hard disable: if parameter is disabled, don't use it at all (even if return is set)
		if !param.Enabled {
			continue
		}

		// Process return override if set
		if param.Return != "" {
			// Process directives in return value ({{file:}} and {{exec:}})
			processedValue, err := directives.ProcessDirectives(param.Return, agentFolder)
			if err != nil {
				// Directive processing failed - return formatted error (match tool error format)
				return fmt.Sprintf(exitCodeFormat, 1, fmt.Sprintf("return directive failed for '%s': %v", paramName, err))
			}
			substitutions[paramName] = processedValue
		}
	}

	// Substitute variables in the command
	cmd := tool.Command
	for key, value := range substitutions {
		placeholder := fmt.Sprintf(placeholderFormat, key)
		cmd = strings.ReplaceAll(cmd, placeholder, value)
	}

	// Log tool execution
	if logger != nil {
		msg := fmt.Sprintf("tool %s", tool.Name)
		if debug {
			logger.Debug(msg, "command", cmd)
		} else {
			logger.Info(msg)
		}
	}

	// Determine working directory
	// Tools run in their own directory (so they can reference local files with ./)
	// Falls back to agent folder if tool has no path (shouldn't happen in practice)
	workDir := agentFolder
	if tool.Path != "" {
		workDir = filepath.Dir(tool.Path)
	}

	// Execute via shell
	stdout, stderr, exitCode, err := shell.Execute(cmd, workDir)

	// Format result
	result := stdout
	if err != nil {
		result = fmt.Sprintf(exitCodeFormat, exitCode, stderr)
	}

	if logger != nil {
		msg := fmt.Sprintf("tool %s completed", tool.Name)
		if verbose {
			logger.Info(msg,
				"exit_code", exitCode,
				"stdout", stdout,
				"stderr", stderr,
			)
		} else {
			logger.Info(msg,
				"exit_code", exitCode,
				"stdout_len", len(stdout),
				"stderr_len", len(stderr),
			)
		}

		// Enhanced debug logging
		if debug {
			debugpkg.LogToolExecution(logger, tool.Name, cmd, result, exitCode)
		}
	}

	return result
}

// FindTool finds a tool by name in the tools list
func FindTool(tools []config.ToolConfig, name string) *config.ToolConfig {
	for i := range tools {
		if tools[i].Name == name {
			return &tools[i]
		}
	}
	return nil
}
