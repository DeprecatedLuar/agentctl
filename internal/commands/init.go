package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/DeprecatedLuar/agentctl/internal/registry"
	"github.com/DeprecatedLuar/agentctl/internal/templates"
)

const (
	// File and directory names
	initAgentConfigFile = "agent.toml"
	promptFile          = "prompt"
	toolsDir            = "tools"
	envFile             = ".env"
	gitignoreFile       = ".gitignore"
	toolExampleFile     = "example.toml"

	// File permissions
	dirPermissions  = 0755
	filePermissions = 0644
)

func HandleInit(args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	// Resolve to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Check if already initialized
	agentTomlPath := filepath.Join(absPath, initAgentConfigFile)
	if _, err := os.Stat(agentTomlPath); err == nil {
		return fmt.Errorf("folder already initialized (%s exists)", initAgentConfigFile)
	}

	// Create directory if doesn't exist
	if err := os.MkdirAll(absPath, dirPermissions); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create tools directory
	toolsPath := filepath.Join(absPath, toolsDir)
	if err := os.MkdirAll(toolsPath, dirPermissions); err != nil {
		return fmt.Errorf("failed to create %s directory: %w", toolsDir, err)
	}

	// Write template files
	files := map[string]string{
		initAgentConfigFile:                 templates.AgentToml,
		promptFile:                          templates.Prompt,
		filepath.Join(toolsDir, toolExampleFile): templates.ToolExample,
		envFile:                             templates.EnvTemplate,
		gitignoreFile:                       templates.Gitignore,
	}

	for name, content := range files {
		filePath := filepath.Join(absPath, name)
		if err := os.WriteFile(filePath, []byte(content), filePermissions); err != nil {
			return fmt.Errorf("failed to write %s: %w", name, err)
		}
	}

	// Register agent in registry
	if err := registry.Register(absPath); err != nil {
		return fmt.Errorf("failed to register agent: %w", err)
	}

	agentName := filepath.Base(absPath)
	fmt.Printf("Initialized agent '%s' at %s\n", agentName, absPath)
	return nil
}
