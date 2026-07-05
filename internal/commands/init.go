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
	configDir   = "config"
	promptsDir  = "prompts"
	toolsDir    = "tools"
	initDataDir = ".data"

	// File names
	initAgentConfig   = "agent.toml"
	contactsFile      = "contacts.toml"
	defaultPromptFile = "default.prompt"
	systemPromptFile  = "system.md"
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
	dirs := []string{configDir, promptsDir, toolsDir, initDataDir}
	for _, dir := range dirs {
		dirPath := filepath.Join(absPath, dir)
		if err := os.MkdirAll(dirPath, dirPermissions); err != nil {
			return fmt.Errorf("failed to create %s directory: %w", dir, err)
		}
	}

	// Write template files
	files := map[string]string{
		filepath.Join(configDir, initAgentConfig):    templates.AgentToml,
		filepath.Join(initDataDir, contactsFile):     substituteUsername(templates.ContactsToml, username),
		filepath.Join(promptsDir, defaultPromptFile): templates.Prompt,
		filepath.Join(promptsDir, systemPromptFile):  templates.SystemMd,
		filepath.Join(toolsDir, toolExampleFile):     templates.ToolExample,
		initEnvFile:                                  templates.EnvTemplate,
		initGitignoreFile:                            templates.Gitignore,
	}

	var createdFiles, skippedFiles []string
	for name, content := range files {
		filePath := filepath.Join(absPath, name)

		// Skip if file already exists
		if _, err := os.Stat(filePath); err == nil {
			skippedFiles = append(skippedFiles, name)
			continue
		}

		if err := os.WriteFile(filePath, []byte(content), filePermissions); err != nil {
			return fmt.Errorf("failed to write %s: %w", name, err)
		}
		createdFiles = append(createdFiles, name)
	}

	// Copy .prerun.sh with executable permissions
	prerunPath := filepath.Join(absPath, ".prerun.sh")
	if _, err := os.Stat(prerunPath); err == nil {
		skippedFiles = append(skippedFiles, ".prerun.sh")
	} else {
		if err := os.WriteFile(prerunPath, []byte(templates.PrerunSh), 0755); err != nil {
			return fmt.Errorf("failed to write .prerun.sh: %w", err)
		}
		createdFiles = append(createdFiles, ".prerun.sh")
	}

	// Register agent in registry
	if err := registry.Register(absPath); err != nil {
		return fmt.Errorf("failed to register agent: %w", err)
	}

	agentName := filepath.Base(absPath)

	// Report results
	if len(createdFiles) > 0 {
		fmt.Printf("Initialized agent '%s' at %s\n", agentName, absPath)
		fmt.Printf("Created %d file(s)\n", len(createdFiles))
	}
	if len(skippedFiles) > 0 {
		fmt.Printf("Skipped %d existing file(s)\n", len(skippedFiles))
	}
	if len(createdFiles) == 0 && len(skippedFiles) > 0 {
		fmt.Printf("Agent '%s' already initialized\n", agentName)
	}

	return nil
}

// substituteUsername replaces {{username}} placeholder with actual username
func substituteUsername(template, username string) string {
	return strings.ReplaceAll(template, "{{username}}", username)
}
