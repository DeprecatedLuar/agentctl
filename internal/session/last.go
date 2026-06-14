package session

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	sessionsDir   = ".data/sessions"
	lastSessionFile = ".last_session"
)

// GetLast returns the last session ID for this user+interface combo.
// Format: interface=sessionID (e.g., "cli=20250614_123456_abc123")
// Path: {agentFolder}/.data/sessions/{userID}/.last_session
// Returns "" if no entry exists.
func GetLast(agentFolder, userID, iface string) (string, error) {
	lastPath := filepath.Join(agentFolder, sessionsDir, userID, lastSessionFile)

	file, err := os.Open(lastPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read .last_session: %w", err)
	}
	defer file.Close()

	// Parse plain text key=value format
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Split on first '='
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue // Skip malformed lines
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if key == iface {
			return value, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to parse .last_session: %w", err)
	}

	return "", nil
}

// SetLast writes/updates the last session ID for this user+interface.
func SetLast(agentFolder, userID, iface, sessionID string) error {
	userDir := filepath.Join(agentFolder, sessionsDir, userID)
	lastPath := filepath.Join(userDir, lastSessionFile)

	// Ensure directory exists
	if err := os.MkdirAll(userDir, 0755); err != nil {
		return fmt.Errorf("failed to create session directory: %w", err)
	}

	// Load existing entries
	entries := make(map[string]string)

	if file, err := os.Open(lastPath); err == nil {
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				entries[key] = value
			}
		}
		file.Close()

		if err := scanner.Err(); err != nil {
			return fmt.Errorf("failed to parse .last_session: %w", err)
		}
	}

	// Update/add entry for this interface
	entries[iface] = sessionID

	// Write back
	file, err := os.OpenFile(lastPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open .last_session: %w", err)
	}
	defer file.Close()

	for k, v := range entries {
		if _, err := fmt.Fprintf(file, "%s=%s\n", k, v); err != nil {
			return fmt.Errorf("failed to write .last_session: %w", err)
		}
	}

	return nil
}
