package hooks

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/DeprecatedLuar/agentctl/internal/shell"
)

// ExecutePrerun runs the prerun script if it exists
// Checks .prerun.sh first (hidden, preferred), then prerun.sh (fallback)
// Non-fatal: logs errors but returns nil to allow agent execution
func ExecutePrerun(agentFolder string, configEnv map[string]string, logger *slog.Logger) error {
	// Check for .prerun.sh first (hidden, preferred)
	prerunPath := filepath.Join(agentFolder, ".prerun.sh")
	scriptName := ".prerun.sh"

	if _, err := os.Stat(prerunPath); os.IsNotExist(err) {
		// Fallback to prerun.sh (visible)
		prerunPath = filepath.Join(agentFolder, "prerun.sh")
		scriptName = "prerun.sh"

		if _, err := os.Stat(prerunPath); os.IsNotExist(err) {
			// Neither exists, skip silently
			return nil
		}
	}

	// Execute script
	stdout, stderr, exitCode, err := shell.Execute(
		"./"+scriptName,
		agentFolder, // workDir
		agentFolder, // agentFolder (.env location)
		configEnv,
		nil, // toolEnv (not a tool execution)
	)

	// Log execution
	if logger != nil {
		if err != nil || exitCode != 0 {
			logger.Warn(scriptName+" failed (continuing anyway)",
				"exit_code", exitCode,
				"stderr", stderr,
			)
		} else {
			logger.Debug(scriptName+" completed",
				"exit_code", exitCode,
				"stdout_len", len(stdout),
			)
		}
	}

	// Non-fatal: always return nil
	return nil
}
