package memory

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/DeprecatedLuar/agentctl/internal/agent"
)

// Message is an alias for agent.Message
type Message = agent.Message

// jsonLine represents a single line in the JSONL file
type jsonLine struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	TS      int64  `json:"ts"`
}

const memoryDir = ".data/memory"

// sessionPath returns the path to a session's JSONL file
func sessionPath(agentFolder, sessionKey string) string {
	filename := sessionKey + ".jsonl"
	return filepath.Join(agentFolder, memoryDir, filename)
}

// Load retrieves the last N messages from a session file in chronological order
func Load(agentFolder, sessionKey string, limit int) ([]Message, error) {
	if limit == 0 {
		return []Message{}, nil
	}

	path := sessionPath(agentFolder, sessionKey)

	// If file doesn't exist, return empty history (no error)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return []Message{}, nil
	}

	// Read all lines from file
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []jsonLine
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var line jsonLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			// Skip malformed lines
			continue
		}
		lines = append(lines, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Take last N lines
	start := 0
	if len(lines) > limit {
		start = len(lines) - limit
	}
	lines = lines[start:]

	// Convert to Messages (already in chronological order)
	messages := make([]Message, len(lines))
	for i, line := range lines {
		messages[i] = Message{
			Role:    line.Role,
			Content: line.Content,
		}
	}

	return messages, nil
}

// Save appends a message to a session file
func Save(agentFolder, sessionKey, role, content string) error {
	// Ensure memory directory exists
	dir := filepath.Join(agentFolder, memoryDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	path := sessionPath(agentFolder, sessionKey)

	// Open file in append mode (create if doesn't exist)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// Create JSON line
	line := jsonLine{
		Role:    role,
		Content: content,
		TS:      time.Now().Unix(),
	}

	data, err := json.Marshal(line)
	if err != nil {
		return err
	}

	// Append line to file
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}

	return nil
}
