package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MintSession generates a new session ID, points userID+gateway at it, and
// creates its backing file so it's immediately writable/injectable even
// before any turn has been saved to it.
func MintSession(store SessionStore, userID, gateway string) (string, error) {
	sessionID := NewSessionID()
	if err := store.SetLast(userID, gateway, sessionID); err != nil {
		return "", fmt.Errorf("failed to set last session: %w", err)
	}
	if err := store.CreateSession(userID, sessionID); err != nil {
		return "", fmt.Errorf("failed to create session file: %w", err)
	}
	return sessionID, nil
}

// Resolve is called by gateways on every incoming message.
// 1. Lazy migration: migrate this contact if they have an unlinked folder
// 2. Calls ResolveUser to get userId
// 3. Calls EnsureContact to log the contact
// 4. Calls GetLast to find existing session or generates new SessionID
// 5. Calls SetLast to persist
// Returns ResolvedSession{UserID, SessionID}
func Resolve(store SessionStore, agentFolder, gateway, platformID, displayName, username string) (ResolvedSession, error) {
	// Step 0: Lazy migration (best-effort, instant if no migration needed)
	_ = MigrateContact(agentFolder, gateway, platformID) // Ignore errors - migration is best-effort

	// Step 1: Resolve user ID
	userID, err := store.ResolveUser(gateway, platformID)
	if err != nil {
		return ResolvedSession{}, fmt.Errorf("failed to resolve user: %w", err)
	}

	// Step 2: Log contact
	if err := store.EnsureContact(gateway, platformID, displayName, username); err != nil {
		return ResolvedSession{}, fmt.Errorf("failed to log contact: %w", err)
	}

	// Step 3: Get or create session ID
	sessionID, err := store.GetLast(userID, gateway)
	if err != nil {
		return ResolvedSession{}, fmt.Errorf("failed to get last session: %w", err)
	}

	if sessionID == "" {
		// No existing session - mint one (also creates its backing file)
		sessionID, err = MintSession(store, userID, gateway)
		if err != nil {
			return ResolvedSession{}, err
		}
	} else {
		// Step 4: Persist as last session
		if err := store.SetLast(userID, gateway, sessionID); err != nil {
			return ResolvedSession{}, fmt.Errorf("failed to set last session: %w", err)
		}
	}

	return ResolvedSession{
		UserID:    userID,
		SessionID: sessionID,
	}, nil
}

// ResolveExplicit is used by chat command when --user/--session flags are set.
// If sessionID is empty, uses last session for that user+gateway.
// If userID is empty, scans all user folders to find which owns the sessionID.
func ResolveExplicit(store SessionStore, agentFolder, userID, sessionID, gateway string) (ResolvedSession, error) {
	// Case 1: Both specified
	if userID != "" && sessionID != "" {
		return ResolvedSession{
			UserID:    userID,
			SessionID: sessionID,
		}, nil
	}

	// Case 2: User specified, session empty - get last session
	if userID != "" && sessionID == "" {
		lastSessionID, err := store.GetLast(userID, gateway)
		if err != nil {
			return ResolvedSession{}, fmt.Errorf("failed to get last session: %w", err)
		}

		if lastSessionID == "" {
			// No existing session - mint one (also creates its backing file)
			lastSessionID, err = MintSession(store, userID, gateway)
			if err != nil {
				return ResolvedSession{}, err
			}
		}

		return ResolvedSession{
			UserID:    userID,
			SessionID: lastSessionID,
		}, nil
	}

	// Case 3: Session specified, user empty - find owner
	if userID == "" && sessionID != "" {
		foundUserID, err := findSessionOwner(agentFolder, sessionID)
		if err != nil {
			return ResolvedSession{}, err
		}

		return ResolvedSession{
			UserID:    foundUserID,
			SessionID: sessionID,
		}, nil
	}

	// Case 4: Both empty - error (shouldn't happen if called correctly)
	return ResolvedSession{}, fmt.Errorf("either userID or sessionID must be specified")
}

// findSessionOwner scans all user folders to find which one owns the given sessionID
func findSessionOwner(agentFolder, sessionID string) (string, error) {
	sessionsRoot := filepath.Join(agentFolder, sessionsDir)

	// Read all user directories
	entries, err := os.ReadDir(sessionsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("session '%s' not found (no sessions exist)", sessionID)
		}
		return "", fmt.Errorf("failed to read sessions directory: %w", err)
	}

	// Search for session file in each user directory
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		userID := entry.Name()
		sessionFile := filepath.Join(sessionsRoot, userID, sessionID+".jsonl")

		if _, err := os.Stat(sessionFile); err == nil {
			// Found it!
			return userID, nil
		}
	}

	return "", fmt.Errorf("session '%s' not found in any user folder", sessionID)
}

// ResolveChannel parses "user@gateway" or "gateway" (user inferred from currentUserID).
// Returns userID, platformID, current sessionID for that user+gateway.
// Creates new session if none exists (consistent with Resolve()).
// currentUserID must be the already-resolved identity ID, not raw platform username.
func ResolveChannel(store SessionStore, agentFolder, channelStr, currentUserID string) (userID, platformID, sessionID string, err error) {
	// Parse channel string
	var gateway string
	if strings.Contains(channelStr, "@") {
		parts := strings.SplitN(channelStr, "@", 2)
		if len(parts) != 2 {
			return "", "", "", fmt.Errorf("invalid channel format (expected 'user@gateway' or 'gateway'): %s", channelStr)
		}
		userID = parts[0]
		gateway = parts[1]
	} else {
		// Bare gateway - use current user
		userID = currentUserID
		gateway = channelStr
	}

	// Lookup platform ID for this user+gateway
	platformID, err = LookupPlatformID(agentFolder, userID, gateway)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to lookup platform ID for %s@%s: %w", userID, gateway, err)
	}

	// Get current session (or create new one)
	sessionID, err = store.GetLast(userID, gateway)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to get last session: %w", err)
	}

	if sessionID == "" {
		// No existing session - mint one (also creates its backing file,
		// consistent with Resolve behavior)
		sessionID, err = MintSession(store, userID, gateway)
		if err != nil {
			return "", "", "", err
		}
	}

	return userID, platformID, sessionID, nil
}

// LookupPlatformID performs reverse lookup from userID to platformID for the given gateway.
// Linear scan of identity's contacts slice — finds entry matching target gateway, extracts platformID.
// Returns error if userID not found in identities or no contact exists for that gateway.
func LookupPlatformID(agentFolder, userID, gateway string) (string, error) {
	// Load identities
	identities, err := LoadIdentities(agentFolder)
	if err != nil {
		return "", fmt.Errorf("failed to load identities: %w", err)
	}

	// Find identity with matching ID
	var identity *Identity
	for i := range identities {
		if identities[i].ID == userID {
			identity = &identities[i]
			break
		}
	}

	if identity == nil {
		return "", fmt.Errorf("user '%s' not found in identities.toml", userID)
	}

	// Scan contacts for matching gateway
	for _, contact := range identity.Contacts {
		contactGateway, contactPlatformID, err := ParseContactKey(contact)
		if err != nil {
			// Skip malformed contacts
			continue
		}

		if contactGateway == gateway {
			// First match wins
			return contactPlatformID, nil
		}
	}

	return "", fmt.Errorf("no %s contact found for user '%s'", gateway, userID)
}
