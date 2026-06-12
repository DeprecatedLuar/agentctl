package agent

import (
	"bytes"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/DeprecatedLuar/agentctl/internal/config"
)

const (
	// Placeholder format
	placeholderFormat = "{{%s}}"

	// Shell execution
	shellCmd     = "sh"
	shellCmdFlag = "-c"

	// Error format
	exitCodeFormat = "exit %d: %s"

	// Default exit code on error
	defaultExitCode = 1
)

// ExecuteTool runs a tool with the given arguments
func ExecuteTool(tool *config.ToolConfig, args map[string]interface{}, agentFolder string, logger *slog.Logger, verbose bool, debug bool) string {
	// Substitute variables in the command
	cmd := tool.Command
	for key, value := range args {
		placeholder := fmt.Sprintf(placeholderFormat, key)
		valueStr := fmt.Sprintf("%v", value)
		cmd = strings.ReplaceAll(cmd, placeholder, valueStr)
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

	// Execute via shell
	execCmd := exec.Command(shellCmd, shellCmdFlag, cmd)

	// Set working directory to tool's directory (so tools can reference local files with ./)
	// Falls back to agent folder if tool has no path (shouldn't happen in practice)
	if tool.Path != "" {
		execCmd.Dir = filepath.Dir(tool.Path)
	} else {
		execCmd.Dir = agentFolder
	}

	var stdout, stderr bytes.Buffer
	execCmd.Stdout = &stdout
	execCmd.Stderr = &stderr

	err := execCmd.Run()

	exitCode := 0
	if err != nil {
		// Get exit code
		exitCode = defaultExitCode
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	// Log tool result
	if logger != nil {
		msg := fmt.Sprintf("tool %s completed", tool.Name)
		if verbose {
			logger.Info(msg,
				"exit_code", exitCode,
				"stdout", stdout.String(),
				"stderr", stderr.String(),
			)
		} else {
			logger.Info(msg,
				"exit_code", exitCode,
				"stdout_len", stdout.Len(),
				"stderr_len", stderr.Len(),
			)
		}
	}

	if err != nil {
		return fmt.Sprintf(exitCodeFormat, exitCode, stderr.String())
	}

	return stdout.String()
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
