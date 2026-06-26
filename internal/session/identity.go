package session

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/DeprecatedLuar/agentctl/internal/config"
)

const (
	// File names
	identitiesFile = ".data/contacts.toml"

	// File permissions
	identitiesFilePermissions = 0644

	// Contact key format
	contactKeySeparator     = ":" // Separator for interface:platformID
	unlinkedFolderSeparator = "-" // Separator for filesystem folder names
	contactKeyExpectedParts = 2   // Expected parts when splitting contact key
)

// Identity represents a user with multiple contact points
type Identity struct {
	ID       string   `toml:"id"`
	Contacts []string `toml:"contacts"` // Format: "interface:platformID"
	Allowed  *bool    `toml:"allowed,omitempty"` // Access control (nil = use default policy)
}

// Contact represents a contact entry (auto-generated)
type Contact struct {
	Interface   string    `toml:"interface"`
	ID          string    `toml:"id"`
	DisplayName string    `toml:"display_name"`
	Username    string    `toml:"username,omitempty"` // Optional: @handle (e.g., Telegram @username)
	FirstSeen   time.Time `toml:"first_seen"`
	Allowed     *bool     `toml:"allowed,omitempty"` // Access control (nil = use default policy)
}

// identitiesConfig represents the merged file structure
type identitiesConfig struct {
	Identities []Identity `toml:"identity"`
	Contacts   []Contact  `toml:"contact"`
}

// ResolveUser returns userId for a given interface+platformID.
// Searches identities.toml for a matching contact string.
// Returns UnlinkedFolderName if no match found (unlinked contact).
func ResolveUser(agentFolder, iface, platformID string) (string, error) {
	identitiesPath := filepath.Join(agentFolder, identitiesFile)

	// Read identities.toml
	data, err := os.ReadFile(identitiesPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No identities file - return unlinked folder name
			return UnlinkedFolderName(iface, platformID), nil
		}
		return "", fmt.Errorf("failed to read identities.toml: %w", err)
	}

	// Parse TOML
	var cfg identitiesConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return "", fmt.Errorf("failed to parse identities.toml: %w", err)
	}

	// Search for matching contact
	contactKey := ContactKey(iface, platformID)
	for _, identity := range cfg.Identities {
		for _, contact := range identity.Contacts {
			if contact == contactKey {
				return identity.ID, nil
			}
		}
	}

	// No match - return unlinked folder name
	return UnlinkedFolderName(iface, platformID), nil
}

// ContactKey returns canonical "interface:platformID" string
func ContactKey(iface, platformID string) string {
	return fmt.Sprintf("%s%s%s", iface, contactKeySeparator, platformID)
}

// UnlinkedFolderName returns filesystem-safe folder name for unlinked contact
// e.g. "telegram:12345678" -> "telegram-12345678"
func UnlinkedFolderName(iface, platformID string) string {
	return fmt.Sprintf("%s%s%s", iface, unlinkedFolderSeparator, platformID)
}

// LoadIdentities loads all identities from contacts.toml
// Returns empty slice if file doesn't exist
func LoadIdentities(agentFolder string) ([]Identity, error) {
	cfg, err := loadIdentitiesFile(agentFolder)
	if err != nil {
		return nil, err
	}
	return cfg.Identities, nil
}

// loadIdentitiesFile loads the entire contacts.toml file (both identities and contacts)
func loadIdentitiesFile(agentFolder string) (*identitiesConfig, error) {
	identitiesPath := filepath.Join(agentFolder, identitiesFile)

	data, err := os.ReadFile(identitiesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &identitiesConfig{}, nil
		}
		return nil, fmt.Errorf("failed to read contacts.toml: %w", err)
	}

	var cfg identitiesConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse contacts.toml: %w", err)
	}

	return &cfg, nil
}

// saveIdentitiesFile writes the entire contacts.toml file (both identities and contacts)
func saveIdentitiesFile(agentFolder string, cfg *identitiesConfig) error {
	identitiesPath := filepath.Join(agentFolder, identitiesFile)

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(identitiesPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := os.OpenFile(identitiesPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, identitiesFilePermissions)
	if err != nil {
		return fmt.Errorf("failed to open contacts.toml: %w", err)
	}
	defer file.Close()

	// Write header comments
	header := `# ============================================
# IDENTITIES (user-edited)
# Link multiple contacts to a single identity
# ============================================

`
	if _, err := file.WriteString(header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Write identities section
	if len(cfg.Identities) > 0 {
		encoder := toml.NewEncoder(file)
		identitiesOnly := struct {
			Identities []Identity `toml:"identity"`
		}{Identities: cfg.Identities}

		if err := encoder.Encode(identitiesOnly); err != nil {
			return fmt.Errorf("failed to write identities: %w", err)
		}
	}

	// Write contacts section header
	contactsHeader := `
# ============================================
# CONTACTS (auto-generated - do not edit)
# All contacts that have sent messages
# ============================================

`
	if _, err := file.WriteString(contactsHeader); err != nil {
		return fmt.Errorf("failed to write contacts header: %w", err)
	}

	// Write contacts section
	if len(cfg.Contacts) > 0 {
		encoder := toml.NewEncoder(file)
		contactsOnly := struct {
			Contacts []Contact `toml:"contact"`
		}{Contacts: cfg.Contacts}

		if err := encoder.Encode(contactsOnly); err != nil {
			return fmt.Errorf("failed to write contacts: %w", err)
		}
	}

	return nil
}

// EnsureContact appends contact entry if not already present.
// Dedup key is interface+id combination.
// Preserves identities section when writing.
func EnsureContact(agentFolder, iface, platformID, displayName, username string) error {
	// Load entire file (both sections)
	cfg, err := loadIdentitiesFile(agentFolder)
	if err != nil {
		return err
	}

	// Check if contact already exists (dedup by interface+id)
	for _, c := range cfg.Contacts {
		if c.Interface == iface && c.ID == platformID {
			// Already exists, no-op
			return nil
		}
	}

	// Load access policy from agent config
	allowByDefault := true // Default to true for backward compatibility
	agentCfg, _ := config.LoadAgent(agentFolder)
	if agentCfg != nil {
		allowByDefault = agentCfg.Access.AllowByDefault
	}

	// Append new contact
	newContact := Contact{
		Interface:   iface,
		ID:          platformID,
		DisplayName: displayName,
		Username:    username,
		FirstSeen:   time.Now(),
		Allowed:     &allowByDefault,
	}

	cfg.Contacts = append(cfg.Contacts, newContact)

	// Write back entire file (preserves identities section)
	return saveIdentitiesFile(agentFolder, cfg)
}

// CheckAccess checks if a contact is allowed to interact with the agent.
// Three-tier precedence: Identity.allowed > Contact.allowed > allow_by_default
// Returns (allowed bool, err error) - false means denial, error means file I/O issue
func CheckAccess(agentFolder, iface, platformID string) (bool, error) {
	// Load contacts file
	cfg, err := loadIdentitiesFile(agentFolder)
	if err != nil {
		return false, fmt.Errorf("failed to load contacts: %w", err)
	}

	// Find contact by interface+platformID
	var contact *Contact
	for i := range cfg.Contacts {
		if cfg.Contacts[i].Interface == iface && cfg.Contacts[i].ID == platformID {
			contact = &cfg.Contacts[i]
			break
		}
	}

	// Contact not found - allow by default (backward compat)
	if contact == nil {
		return true, nil
	}

	// Check if contact is linked to an identity
	contactKey := ContactKey(iface, platformID)
	for _, identity := range cfg.Identities {
		for _, identityContact := range identity.Contacts {
			if identityContact == contactKey {
				// Found identity - check identity.allowed first
				if identity.Allowed != nil {
					return *identity.Allowed, nil
				}
				// Identity has no allowed field - fall through to contact check
				break
			}
		}
	}

	// No identity override - check contact.allowed
	if contact.Allowed != nil {
		return *contact.Allowed, nil
	}

	// No explicit setting - allow by default (backward compat)
	return true, nil
}

// ParseContactKey splits a contact key "interface:platformID" into components
func ParseContactKey(contactKey string) (iface, platformID string, err error) {
	parts := strings.SplitN(contactKey, contactKeySeparator, contactKeyExpectedParts)
	if len(parts) != contactKeyExpectedParts {
		return "", "", fmt.Errorf("invalid contact key format (expected 'interface%splatformID'): %s", contactKeySeparator, contactKey)
	}
	return parts[0], parts[1], nil
}

// WarnDuplicateContacts scans identities.toml for duplicate interface contacts.
// Logs warning once at startup if any identity has multiple contacts for the same interface.
// First match wins in LookupPlatformID when duplicates exist.
func WarnDuplicateContacts(agentFolder string, logger *slog.Logger) error {
	identities, err := LoadIdentities(agentFolder)
	if err != nil {
		return err
	}

	for _, identity := range identities {
		// Track seen interfaces
		seen := make(map[string]bool)
		warned := make(map[string]bool)

		for _, contact := range identity.Contacts {
			iface, _, err := ParseContactKey(contact)
			if err != nil {
				// Skip malformed contacts
				continue
			}

			if seen[iface] && !warned[iface] {
				// Found duplicate - log warning once per interface
				logger.Warn("identity has multiple contacts for same interface, using first match",
					"identity", identity.ID,
					"interface", iface)
				warned[iface] = true
			}

			seen[iface] = true
		}
	}

	return nil
}
