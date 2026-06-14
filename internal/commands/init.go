package commands

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/DeprecatedLuar/agentctl/internal/registry"
	"github.com/DeprecatedLuar/agentctl/internal/templates"
)

const (
	// Directory names
	configDir  = "config"
	promptsDir = "prompts"
	toolsDir   = "tools"

	// File names
	initAgentConfig   = "agent.toml"
	identitiesFile    = "identities.toml"
	defaultPromptFile = "default"
	initEnvFile       = ".env"
	initGitignoreFile = ".gitignore"
	toolExampleFile   = "example.toml"

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

	// Check if already initialized (check for config/agent.toml)
	agentTomlPath := filepath.Join(absPath, configDir, initAgentConfig)
	if _, err := os.Stat(agentTomlPath); err == nil {
		return fmt.Errorf("folder already initialized (%s exists)", filepath.Join(configDir, initAgentConfig))
	}

	// Get current username for template substitution
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}
	username := currentUser.Username

	// Create directory if doesn't exist
	if err := os.MkdirAll(absPath, dirPermissions); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create subdirectories
	dirs := []string{configDir, promptsDir, toolsDir}
	for _, dir := range dirs {
		dirPath := filepath.Join(absPath, dir)
		if err := os.MkdirAll(dirPath, dirPermissions); err != nil {
			return fmt.Errorf("failed to create %s directory: %w", dir, err)
		}
	}

	// Write template files
	files := map[string]string{
		filepath.Join(configDir, initAgentConfig):    templates.AgentToml,
		filepath.Join(configDir, identitiesFile):     substituteUsername(templates.IdentitiesToml, username),
		filepath.Join(promptsDir, defaultPromptFile): templates.Prompt,
		filepath.Join(toolsDir, toolExampleFile):     templates.ToolExample,
		initEnvFile:                                   templates.EnvTemplate,
		initGitignoreFile:                             templates.Gitignore,
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

// substituteUsername replaces {{username}} placeholder with actual username
func substituteUsername(template, username string) string {
	return strings.ReplaceAll(template, "{{username}}", username)
}
