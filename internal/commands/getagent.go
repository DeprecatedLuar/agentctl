package commands

import (
	"fmt"
	"path/filepath"

	"github.com/DeprecatedLuar/agentctl/internal/registry"
)

const (
	flagAgentLong  = "--agent"
	flagAgentShort = "-a"
)

// HandleGetAgent returns the name of the current agent folder.
// With no flags: walks upward from CWD to find agent.toml
// With -a flag: resolves the provided name/path
// Returns just the agent name (basename of the folder) to stdout.
func HandleGetAgent(args []string) error {
	var agentPath string
	var err error

	// Parse -a/--agent flag
	if len(args) > 0 && (args[0] == flagAgentLong || args[0] == flagAgentShort) {
		if len(args) < 2 {
			return fmt.Errorf("%s requires a path or name", args[0])
		}
		// Resolve provided name/path
		agentPath, err = registry.ResolveAgentPath(args[1])
		if err != nil {
			return err
		}
	} else {
		// No flags: find agent in current path
		agentPath, err = registry.FindAgentInPath()
		if err != nil {
			return err
		}
	}

	// Output just the agent name (basename)
	agentName := filepath.Base(agentPath)
	fmt.Println(agentName)
	return nil
}
