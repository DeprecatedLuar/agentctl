package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/DeprecatedLuar/agentctl/internal/config"
)

func HandleRun(args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	// Resolve to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Validate agent folder
	agentTomlPath := filepath.Join(absPath, "agent.toml")
	if _, err := os.Stat(agentTomlPath); err != nil {
		return fmt.Errorf("not an agent folder (agent.toml not found)")
	}

	// Load config
	agentCfg, err := config.LoadAgent(absPath)
	if err != nil {
		return err
	}

	// Load tools
	tools, err := config.LoadTools(absPath, agentCfg.Tools)
	if err != nil {
		return err
	}

	// Print summary
	fmt.Printf("Agent Config:\n")
	fmt.Printf("  Provider: %s\n", agentCfg.Provider)
	fmt.Printf("  Model: %s\n", agentCfg.Model)
	fmt.Printf("  Memory: max_messages=%d\n", agentCfg.Memory.MaxMessages)
	fmt.Printf("\nTools loaded: %d\n", len(tools))
	for _, tool := range tools {
		fmt.Printf("  - %s: %s\n", tool.Name, tool.Command)
		if len(tool.Parameters) > 0 {
			fmt.Printf("    Parameters:\n")
			for name, param := range tool.Parameters {
				required := ""
				if param.Required {
					required = " (required)"
				}
				fmt.Printf("      - %s (%s)%s\n", name, param.Type, required)
			}
		}
	}

	return nil
}
