package session

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// Directory and file names
	sessionsDir     = ".data/sessions"
	lastSessionFile = ".last_session"
	jsonlExtension  = ".jsonl"

	// Metadata
	metaType = "meta"

	// File permissions
	sessionDirPermissions  = 0755
	sessionFilePermissions = 0644

	// Temporary file patterns
	sessionTempPattern     = ".session-*.jsonl.tmp"
	lastSessionTempPattern = ".last_session.*.tmp"

	// Parsing
	lastSessionSeparator    = "="
	lastSessionExpectedParts = 2
)

// JSONLStore implements SessionStore using JSONL files
type JSONLStore struct {
	AgentFolder  string
	sessionLocks sync.Map // key: "userID/sessionID", value: *sync.Mutex
}

// NewJSONLStore creates a new JSONL-based session store
func NewJSONLStore(agentFolder string) *JSONLStore {
	return &JSONLStore{AgentFolder: agentFolder}
}

// metaLine represents the metadata line (first line of JSONL)
type metaLine struct {
	Type      string `json:"type"` // Always "meta"
	Title     string `json:"title"`
	CreatedAt int64  `json:"created_at"`
	Interface string `json:"interface"`
	UserID    string `json:"user_id"`
}

// sessionFilePath returns the path to a session's JSONL file
func (s *JSONLStore) sessionFilePath(userID, sessionID string) string {
	return filepath.Join(s.AgentFolder, sessionsDir, userID, sessionID+jsonlExtension)
}

// UserFolder returns the per-user session directory path.
func UserFolder(agentFolder, userID string) string {
	return filepath.Join(agentFolder, sessionsDir, userID)
}

// getSessionLock returns the mutex for a specific session, creating it if needed.
// This ensures all operations on the same session file are serialized.
func (s *JSONLStore) getSessionLock(userID, sessionID string) *sync.Mutex {
	key := userID + "/" + sessionID
	lock, _ := s.sessionLocks.LoadOrStore(key, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

// Load retrieves the last N messages from a session file in chronological order.
// Skips the first line (metadata) if present.
func (s *JSONLStore) Load(userID, sessionID string, limit int) ([]Message, error) {
	if limit == 0 {
		return []Message{}, nil
	}

	// Lock session file for reading
	lock := s.getSessionLock(userID, sessionID)
	lock.Lock()
	defer lock.Unlock()

	path := s.sessionFilePath(userID, sessionID)

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
	lineNum := 0
	for scanner.Scan() {
		lineNum++

		// Skip first line if it's metadata
		if lineNum == 1 {
			var meta map[string]interface{}
			if err := json.Unmarshal(scanner.Bytes(), &meta); err == nil {
				if metaTypeValue, ok := meta["type"].(string); ok && metaTypeValue == metaType {
					continue // Skip metadata line
				}
			}
			// If first line is not metadata, fall through to parse as message
		}

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
// On new session (file does not exist), writes metadata line first.
func (s *JSONLStore) Save(userID, sessionID, role, content string) error {
	// Lock session file for writing
	lock := s.getSessionLock(userID, sessionID)
	lock.Lock()
	defer lock.Unlock()

	// Ensure session directory exists
	sessionDir := filepath.Join(s.AgentFolder, sessionsDir, userID)
	if err := os.MkdirAll(sessionDir, sessionDirPermissions); err != nil {
		return fmt.Errorf("failed to create session directory: %w", err)
	}

	path := s.sessionFilePath(userID, sessionID)

	// Check if this is a new session
	isNewSession := false
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		isNewSession = true
	}

	// Open file in append mode (create if doesn't exist)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, sessionFilePermissions)
	if err != nil {
		return fmt.Errorf("failed to open session file: %w", err)
	}
	defer file.Close()

	// Write metadata line for new sessions
	if isNewSession {
		meta := metaLine{
			Type:      metaType,
			Title:     "", // Empty initially, will be set by auto-title
			CreatedAt: time.Now().Unix(),
			Interface: "", // Will be set when we know the interface
			UserID:    userID,
		}

		metaData, err := json.Marshal(meta)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}

		if _, err := file.Write(append(metaData, '\n')); err != nil {
			return fmt.Errorf("failed to write metadata: %w", err)
		}
	}

	// Create and append message line
	line := Message{
		Role:    role,
		Content: content,
		TS:      time.Now().Unix(),
	}

	data, err := json.Marshal(line)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	if _, err := file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write to session file: %w", err)
	}

	return nil
}

// GetMeta retrieves session metadata from the first line of the JSONL file.
// Returns empty metadata if file doesn't exist or first line is not metadata.
func (s *JSONLStore) GetMeta(userID, sessionID string) (SessionMeta, error) {
	// Lock session file for reading
	lock := s.getSessionLock(userID, sessionID)
	lock.Lock()
	defer lock.Unlock()

	path := s.sessionFilePath(userID, sessionID)

	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Return empty metadata for non-existent sessions
			return SessionMeta{}, nil
		}
		return SessionMeta{}, fmt.Errorf("failed to open session file: %w", err)
	}
	defer file.Close()

	// Read first line only
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		// Empty file
		return SessionMeta{}, nil
	}

	var meta metaLine
	if err := json.Unmarshal(scanner.Bytes(), &meta); err != nil {
		// First line is not valid JSON or metadata
		return SessionMeta{}, nil
	}

	// Check if it's a metadata line
	if meta.Type != metaType {
		// Old session format without metadata
		return SessionMeta{}, nil
	}

	return SessionMeta{
		Title:     meta.Title,
		CreatedAt: meta.CreatedAt,
		Interface: meta.Interface,
		UserID:    meta.UserID,
	}, nil
}

// SetMeta updates session metadata atomically.
// Reads entire file, replaces first line, writes to temp file, then renames.
func (s *JSONLStore) SetMeta(userID, sessionID string, meta SessionMeta) error {
	// Lock session file for writing
	lock := s.getSessionLock(userID, sessionID)
	lock.Lock()
	defer lock.Unlock()

	path := s.sessionFilePath(userID, sessionID)

	// Read all lines
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open session file: %w", err)
	}

	var lines []string
	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum == 1 {
			// Skip first line, we'll replace it
			continue
		}
		lines = append(lines, scanner.Text())
	}
	file.Close()

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read session file: %w", err)
	}

	// Create new metadata line
	newMeta := metaLine{
		Type:      metaType,
		Title:     meta.Title,
		CreatedAt: meta.CreatedAt,
		Interface: meta.Interface,
		UserID:    meta.UserID,
	}

	metaData, err := json.Marshal(newMeta)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Write to temp file
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, sessionTempPattern)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Write metadata line first
	if _, err := tmpFile.Write(append(metaData, '\n')); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	// Write remaining lines
	for _, line := range lines {
		if _, err := tmpFile.Write([]byte(line + "\n")); err != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("failed to write line: %w", err)
		}
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// GetLast returns the last session ID for this user+interface combo.
func (s *JSONLStore) GetLast(userID, iface string) (string, error) {
	lastPath := filepath.Join(s.AgentFolder, sessionsDir, userID, lastSessionFile)

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

		// Split on first separator
		parts := strings.SplitN(line, lastSessionSeparator, lastSessionExpectedParts)
		if len(parts) != lastSessionExpectedParts {
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

// SetLast updates the last session ID for this user+interface atomically.
func (s *JSONLStore) SetLast(userID, iface, sessionID string) error {
	userDir := filepath.Join(s.AgentFolder, sessionsDir, userID)
	lastPath := filepath.Join(userDir, lastSessionFile)

	// Ensure directory exists
	if err := os.MkdirAll(userDir, sessionDirPermissions); err != nil {
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

			parts := strings.SplitN(line, lastSessionSeparator, lastSessionExpectedParts)
			if len(parts) == lastSessionExpectedParts {
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

	// Write to temp file
	tmpFile, err := os.CreateTemp(userDir, lastSessionTempPattern)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	for k, v := range entries {
		if _, err := fmt.Fprintf(tmpFile, "%s=%s\n", k, v); err != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("failed to write .last_session: %w", err)
		}
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, lastPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// ListSessions returns all session metadata for a user.
func (s *JSONLStore) ListSessions(userID string) ([]SessionMeta, error) {
	userDir := filepath.Join(s.AgentFolder, sessionsDir, userID)

	entries, err := os.ReadDir(userDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []SessionMeta{}, nil
		}
		return nil, fmt.Errorf("failed to read sessions directory: %w", err)
	}

	var sessions []SessionMeta
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), jsonlExtension) {
			continue
		}

		// Extract session ID from filename
		sessionID := strings.TrimSuffix(entry.Name(), jsonlExtension)

		// Load metadata
		meta, err := s.GetMeta(userID, sessionID)
		if err != nil {
			// Skip sessions with read errors
			continue
		}

		// Populate SessionID from filename (not stored in JSONL metadata)
		meta.SessionID = sessionID

		sessions = append(sessions, meta)
	}

	return sessions, nil
}

// EnsureContact delegates to the contacts.go implementation
func (s *JSONLStore) EnsureContact(iface, platformID, displayName, username string) error {
	return EnsureContact(s.AgentFolder, iface, platformID, displayName, username)
}

// ResolveUser delegates to the identity.go implementation
func (s *JSONLStore) ResolveUser(iface, platformID string) (string, error) {
	return ResolveUser(s.AgentFolder, iface, platformID)
}

// SessionExists checks if a session file exists
func (s *JSONLStore) SessionExists(userID, sessionID string) bool {
	path := s.sessionFilePath(userID, sessionID)
	_, err := os.Stat(path)
	return err == nil
}

// CreateSession ensures a session file exists on disk with its metadata line,
// without appending any message. No-op if the file already exists. Used so a
// freshly-minted session ID is immediately writable/injectable even before
// any turn has been saved to it.
func (s *JSONLStore) CreateSession(userID, sessionID string) error {
	lock := s.getSessionLock(userID, sessionID)
	lock.Lock()
	defer lock.Unlock()

	sessionDir := filepath.Join(s.AgentFolder, sessionsDir, userID)
	if err := os.MkdirAll(sessionDir, sessionDirPermissions); err != nil {
		return fmt.Errorf("failed to create session directory: %w", err)
	}

	path := s.sessionFilePath(userID, sessionID)
	if _, err := os.Stat(path); err == nil {
		return nil // already exists
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, sessionFilePermissions)
	if err != nil {
		return fmt.Errorf("failed to create session file: %w", err)
	}
	defer file.Close()

	meta := metaLine{
		Type:      metaType,
		Title:     "",
		CreatedAt: time.Now().Unix(),
		Interface: "",
		UserID:    userID,
	}

	metaData, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if _, err := file.Write(append(metaData, '\n')); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	return nil
}
