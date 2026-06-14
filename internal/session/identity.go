package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const identitiesFile = "config/identities.toml"

// Identity represents a user with multiple contact points
type Identity struct {
	ID       string   `toml:"id"`
	Contacts []string `toml:"contacts"` // Format: "interface:platformID"
}

type identitiesConfig struct {
	Identities []Identity `toml:"identity"`
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
	return fmt.Sprintf("%s:%s", iface, platformID)
}

// UnlinkedFolderName returns filesystem-safe folder name for unlinked contact
// e.g. "telegram:12345678" -> "telegram-12345678"
func UnlinkedFolderName(iface, platformID string) string {
	return fmt.Sprintf("%s-%s", iface, platformID)
}

// LoadIdentities loads all identities from identities.toml
// Returns empty slice if file doesn't exist
func LoadIdentities(agentFolder string) ([]Identity, error) {
	identitiesPath := filepath.Join(agentFolder, identitiesFile)

	data, err := os.ReadFile(identitiesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []Identity{}, nil
		}
		return nil, fmt.Errorf("failed to read identities.toml: %w", err)
	}

	var cfg identitiesConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse identities.toml: %w", err)
	}

	return cfg.Identities, nil
}

// ParseContactKey splits a contact key "interface:platformID" into components
func ParseContactKey(contactKey string) (iface, platformID string, err error) {
	parts := strings.SplitN(contactKey, ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid contact key format (expected 'interface:platformID'): %s", contactKey)
	}
	return parts[0], parts[1], nil
}
