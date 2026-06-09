package agent

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/DeprecatedLuar/agentctl/internal/config"
)

// ExecuteTool runs a tool with the given arguments
func ExecuteTool(tool *config.ToolConfig, args map[string]interface{}) string {
	// Substitute variables in the command
	cmd := tool.Command
	for key, value := range args {
		placeholder := fmt.Sprintf("{{%s}}", key)
		valueStr := fmt.Sprintf("%v", value)
		cmd = strings.ReplaceAll(cmd, placeholder, valueStr)
	}

	// Execute via shell
	execCmd := exec.Command("sh", "-c", cmd)
	var stdout, stderr bytes.Buffer
	execCmd.Stdout = &stdout
	execCmd.Stderr = &stderr

	err := execCmd.Run()
	if err != nil {
		// Return error with exit code and stderr
		exitCode := 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		return fmt.Sprintf("exit %d: %s", exitCode, stderr.String())
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
