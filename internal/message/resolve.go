// Package message resolves free-form text input for CLI commands (chat,
// deliver, inject) from a flag, an environment variable, or an interactive
// editor, so callers never need to pass arbitrary text as a positional
// shell argument.
package message

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/term"
)

// EnvVar is the environment variable holding the resolved message text.
const EnvVar = "AGENTCTL_MESSAGE"

// Resolve ensures EnvVar is populated, in order:
// flagValue (if flagGiven) > pre-set env var > interactive editor.
func Resolve(flagValue string, flagGiven bool) (string, error) {
	if flagGiven {
		os.Setenv(EnvVar, flagValue)
	}
	text := os.Getenv(EnvVar)
	if text != "" {
		return text, nil
	}
	if !isInteractive() {
		return "", fmt.Errorf("message is required")
	}
	content, err := readFromEditor()
	if err != nil {
		return "", err
	}
	os.Setenv(EnvVar, content)
	return content, nil
}

// isInteractive reports whether stdin is an interactive terminal (as
// opposed to a pipe, a redirect from a file, or /dev/null under cron/hooks).
func isInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// readFromEditor opens $EDITOR (fallback vi) on a temp file and returns its
// trimmed contents once the editor exits.
func readFromEditor() (string, error) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	f, err := os.CreateTemp("", "agentctl-message-*.txt")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	path := f.Name()
	f.Close()
	defer os.Remove(path)

	cmd := exec.Command(editor, path)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("editor exited with error: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read message file: %w", err)
	}
	content := strings.TrimRight(string(data), "\n")
	if content == "" {
		return "", fmt.Errorf("message is required")
	}
	return content, nil
}
