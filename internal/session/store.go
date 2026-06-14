package session

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Message represents a stored message in the session history
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	TS      int64  `json:"ts"`
}

// sessionFilePath returns the path to a session's JSONL file
// Path: {agentFolder}/.data/sessions/{userID}/{sessionID}.jsonl
func sessionFilePath(agentFolder, userID, sessionID string) string {
	return filepath.Join(agentFolder, sessionsDir, userID, sessionID+".jsonl")
}

// Load retrieves the last N messages from a session file in chronological order.
// Save uses os.MkdirAll to create session directory if missing.
func Load(agentFolder, userID, sessionID string, limit int) ([]Message, error) {
	if limit == 0 {
		return []Message{}, nil
	}

	path := sessionFilePath(agentFolder, userID, sessionID)

	// If file doesn't exist, return empty history (no error)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return []Message{}, nil
	}

	// Read all lines from file
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open session file: %w", err)
	}
	defer file.Close()

	var lines []Message
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var line Message
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			// Skip malformed lines
			continue
		}
		lines = append(lines, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	// Take last N lines
	start := 0
	if len(lines) > limit {
		start = len(lines) - limit
	}
	lines = lines[start:]

	return lines, nil
}

// Save appends a message to a session file.
// Uses os.MkdirAll to create session directory if missing.
func Save(agentFolder, userID, sessionID, role, content string) error {
	// Ensure session directory exists
	sessionDir := filepath.Join(agentFolder, sessionsDir, userID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return fmt.Errorf("failed to create session directory: %w", err)
	}

	path := sessionFilePath(agentFolder, userID, sessionID)

	// Open file in append mode (create if doesn't exist)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open session file: %w", err)
	}
	defer file.Close()

	// Create JSON line
	line := Message{
		Role:    role,
		Content: content,
		TS:      time.Now().Unix(),
	}

	data, err := json.Marshal(line)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Append line to file
	if _, err := file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write to session file: %w", err)
	}

	return nil
}
