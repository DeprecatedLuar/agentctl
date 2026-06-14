package session

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MigrateOnStartup reads identities.toml and for each identity,
// checks if any of its contacts still have their own unlinked folder.
// If found, renames all session files into the named identity folder.
// Merges .last_session by keeping the most recent timestamp per interface.
// Safe to call multiple times (idempotent).
func MigrateOnStartup(agentFolder string) error {
	identities, err := LoadIdentities(agentFolder)
	if err != nil {
		return fmt.Errorf("failed to load identities: %w", err)
	}

	sessionsRoot := filepath.Join(agentFolder, sessionsDir)

	// Process each identity
	for _, identity := range identities {
		identityDir := filepath.Join(sessionsRoot, identity.ID)

		// Process each contact in this identity
		for _, contactKey := range identity.Contacts {
			iface, platformID, err := ParseContactKey(contactKey)
			if err != nil {
				// Skip malformed contact keys
				continue
			}

			// Check if unlinked folder exists
			unlinkedFolder := UnlinkedFolderName(iface, platformID)
			unlinkedPath := filepath.Join(sessionsRoot, unlinkedFolder)

			if _, err := os.Stat(unlinkedPath); os.IsNotExist(err) {
				// No unlinked folder - nothing to migrate
				continue
			}

			// Migrate session files from unlinked folder to identity folder
			if err := migrateFolder(unlinkedPath, identityDir); err != nil {
				return fmt.Errorf("failed to migrate %s to %s: %w", unlinkedFolder, identity.ID, err)
			}

			// Remove the now-empty unlinked folder
			if err := os.RemoveAll(unlinkedPath); err != nil {
				return fmt.Errorf("failed to remove unlinked folder %s: %w", unlinkedFolder, err)
			}
		}
	}

	return nil
}

// migrateFolder moves all session files from srcDir to dstDir.
// Merges .last_session files by keeping most recent session per interface.
func migrateFolder(srcDir, dstDir string) error {
	// Ensure destination directory exists
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("failed to read source directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())

		// Special handling for .last_session
		if entry.Name() == lastSessionFile {
			if err := mergeLastSession(srcPath, dstPath); err != nil {
				return fmt.Errorf("failed to merge .last_session: %w", err)
			}
			continue
		}

		// Move regular session files (.jsonl)
		if err := os.Rename(srcPath, dstPath); err != nil {
			return fmt.Errorf("failed to move %s: %w", entry.Name(), err)
		}
	}

	return nil
}

// mergeLastSession merges two .last_session files, keeping the most recent
// session ID per interface (based on timestamp in session ID).
func mergeLastSession(srcPath, dstPath string) error {
	srcEntries, err := parseLastSession(srcPath)
	if err != nil {
		return fmt.Errorf("failed to parse source .last_session: %w", err)
	}

	dstEntries, err := parseLastSession(dstPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to parse destination .last_session: %w", err)
	}

	// Merge: keep most recent session per interface
	merged := make(map[string]string)
	for k, v := range dstEntries {
		merged[k] = v
	}

	for iface, srcSessionID := range srcEntries {
		dstSessionID, exists := merged[iface]
		if !exists || isMoreRecent(srcSessionID, dstSessionID) {
			merged[iface] = srcSessionID
		}
	}

	// Write merged result
	return writeLastSession(dstPath, merged)
}

// parseLastSession reads a .last_session file and returns interface->sessionID map
func parseLastSession(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	entries := make(map[string]string)
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

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

// writeLastSession writes interface->sessionID map to .last_session file
func writeLastSession(path string, entries map[string]string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	for iface, sessionID := range entries {
		if _, err := fmt.Fprintf(file, "%s=%s\n", iface, sessionID); err != nil {
			return err
		}
	}

	return nil
}

// isMoreRecent returns true if sessionID1 has a more recent timestamp than sessionID2.
// Session ID format: YYYYMMDD_HHMMSS_<6-hex>
func isMoreRecent(sessionID1, sessionID2 string) bool {
	ts1, err1 := parseSessionTimestamp(sessionID1)
	ts2, err2 := parseSessionTimestamp(sessionID2)

	// If either parse fails, consider sessionID1 more recent (prefer migration source)
	if err1 != nil || err2 != nil {
		return true
	}

	return ts1.After(ts2)
}

// parseSessionTimestamp extracts timestamp from session ID
func parseSessionTimestamp(sessionID string) (time.Time, error) {
	// Format: YYYYMMDD_HHMMSS_<6-hex>
	// Extract first 15 chars: YYYYMMDD_HHMMSS
	if len(sessionID) < 15 {
		return time.Time{}, fmt.Errorf("invalid session ID format")
	}

	timestampStr := sessionID[:15]
	return time.Parse("20060102_150405", timestampStr)
}

// MigrateContact migrates a single contact's session folder if it exists as unlinked.
// Called lazily on each message - fast path if no migration needed (single stat check).
// Returns nil if no migration needed (already migrated or never existed).
func MigrateContact(agentFolder, iface, platformID string) error {
	// Check if unlinked folder exists (fast path: single stat syscall)
	unlinkedFolder := UnlinkedFolderName(iface, platformID)
	unlinkedPath := filepath.Join(agentFolder, sessionsDir, unlinkedFolder)

	if _, err := os.Stat(unlinkedPath); os.IsNotExist(err) {
		return nil // Already migrated or never existed - instant return
	}

	// Find which identity owns this contact
	userID, err := ResolveUser(agentFolder, iface, platformID)
	if err != nil {
		return err
	}

	// If still resolves to unlinked name, no identity owns it - nothing to migrate
	if userID == unlinkedFolder {
		return nil
	}

	// Migrate this contact's folder to the identity folder
	identityPath := filepath.Join(agentFolder, sessionsDir, userID)
	if err := migrateFolder(unlinkedPath, identityPath); err != nil {
		return fmt.Errorf("failed to migrate %s to %s: %w", unlinkedFolder, userID, err)
	}

	// Remove the unlinked folder
	if err := os.RemoveAll(unlinkedPath); err != nil {
		return fmt.Errorf("failed to remove unlinked folder %s: %w", unlinkedFolder, err)
	}

	return nil
}
