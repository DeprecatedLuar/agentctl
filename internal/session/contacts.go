package session

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

const (
	// File names
	contactsFile = "contacts.toml"

	// File permissions
	contactsFilePermissions = 0644
)

// Contact represents a contact entry in contacts.toml
type Contact struct {
	Interface   string    `toml:"interface"`
	ID          string    `toml:"id"`
	DisplayName string    `toml:"display_name"`
	FirstSeen   time.Time `toml:"first_seen"`
}

type contactsConfig struct {
	Contacts []Contact `toml:"contact"`
}

// EnsureContact appends contact entry if not already present.
// Dedup key is interface+id combination.
// Creates contacts.toml via os.OpenFile with O_CREATE|O_APPEND if missing.
func EnsureContact(agentFolder, iface, platformID, displayName string) error {
	contactsPath := filepath.Join(agentFolder, contactsFile)

	// Load existing contacts
	existing, err := loadContacts(contactsPath)
	if err != nil {
		return err
	}

	// Check if contact already exists (dedup by interface+id)
	for _, c := range existing {
		if c.Interface == iface && c.ID == platformID {
			// Already exists, no-op
			return nil
		}
	}

	// Append new contact
	newContact := Contact{
		Interface:   iface,
		ID:          platformID,
		DisplayName: displayName,
		FirstSeen:   time.Now(),
	}

	existing = append(existing, newContact)

	// Write back to file
	return saveContacts(contactsPath, existing)
}

// loadContacts reads contacts.toml and returns all contacts
// Returns empty slice if file doesn't exist
func loadContacts(path string) ([]Contact, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Contact{}, nil
		}
		return nil, fmt.Errorf("failed to read contacts.toml: %w", err)
	}

	var cfg contactsConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse contacts.toml: %w", err)
	}

	return cfg.Contacts, nil
}

// saveContacts writes contacts to contacts.toml
func saveContacts(path string, contacts []Contact) error {
	cfg := contactsConfig{Contacts: contacts}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, contactsFilePermissions)
	if err != nil {
		return fmt.Errorf("failed to open contacts.toml: %w", err)
	}
	defer file.Close()

	encoder := toml.NewEncoder(file)
	if err := encoder.Encode(cfg); err != nil {
		return fmt.Errorf("failed to write contacts.toml: %w", err)
	}

	return nil
}
