package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/DeprecatedLuar/agentctl/internal/templates"
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
	agentTomlPath := filepath.Join(absPath, "agent.toml")
	if _, err := os.Stat(agentTomlPath); err == nil {
		return fmt.Errorf("folder already initialized (agent.toml exists)")
	}

	// Create directory if doesn't exist
	if err := os.MkdirAll(absPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create tools directory
	toolsPath := filepath.Join(absPath, "tools")
	if err := os.MkdirAll(toolsPath, 0755); err != nil {
		return fmt.Errorf("failed to create tools directory: %w", err)
	}

	// Write template files
	files := map[string]string{
		"agent.toml":         templates.AgentToml,
		"prompt":             templates.Prompt,
		"tools/example.toml": templates.ToolExample,
		".env.template":      templates.EnvTemplate,
		".gitignore":         templates.Gitignore,
	}

	for name, content := range files {
		filePath := filepath.Join(absPath, name)
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", name, err)
		}
	}

	fmt.Printf("Initialized agent folder at %s\n", absPath)
	return nil
}
