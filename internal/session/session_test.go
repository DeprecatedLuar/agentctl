package session

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

func TestNewSessionID(t *testing.T) {
	// Format: YYYYMMDD_HHMMSS_<6-char-hex>
	pattern := regexp.MustCompile(`^\d{8}_\d{6}_[a-f0-9]{6}$`)

	id := NewSessionID()

	if !pattern.MatchString(id) {
		t.Errorf("NewSessionID() = %q, want format YYYYMMDD_HHMMSS_[a-f0-9]{6}", id)
	}

	// Verify timestamp component is recent
	now := time.Now()
	expectedPrefix := now.Format("20060102_1504")
	if id[:13] != expectedPrefix {
		t.Errorf("NewSessionID() timestamp = %s, want %s", id[:13], expectedPrefix)
	}

	// Test uniqueness
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := NewSessionID()
		if ids[id] {
			t.Errorf("NewSessionID() generated duplicate: %s", id)
		}
		ids[id] = true
	}
}

func TestResolveUser(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .data directory and contacts.toml with identities
	dataDir := filepath.Join(tmpDir, ".data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}

	identitiesContent := `[[identity]]
id = "alice"
contacts = ["cli:alice", "telegram:12345"]

[[identity]]
id = "bob"
contacts = ["cli:bob"]
`
	identitiesPath := filepath.Join(dataDir, "contacts.toml")
	if err := os.WriteFile(identitiesPath, []byte(identitiesContent), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		iface      string
		platformID string
		want       string
	}{
		{"known CLI contact", "cli", "alice", "alice"},
		{"known Telegram contact", "telegram", "12345", "alice"},
		{"known CLI contact bob", "cli", "bob", "bob"},
		{"unknown contact", "telegram", "99999", "telegram-99999"},
		{"unknown interface", "discord", "user123", "discord-user123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveUser(tmpDir, tt.iface, tt.platformID)
			if err != nil {
				t.Fatalf("ResolveUser() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("ResolveUser(%s, %s) = %q, want %q", tt.iface, tt.platformID, got, tt.want)
			}
		})
	}
}

func TestResolveUser_NoIdentitiesFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Don't create identities.toml - should return unlinked folder name
	userID, err := ResolveUser(tmpDir, "cli", "alice")
	if err != nil {
		t.Fatalf("ResolveUser() error = %v", err)
	}

	want := "cli-alice"
	if userID != want {
		t.Errorf("ResolveUser() = %q, want %q", userID, want)
	}
}

func TestEnsureContact(t *testing.T) {
	tmpDir := t.TempDir()

	// First call - should create file
	err := EnsureContact(tmpDir, "cli", "alice", "Alice")
	if err != nil {
		t.Fatalf("EnsureContact() first call error = %v", err)
	}

	// Verify file was created
	contactsPath := filepath.Join(tmpDir, identitiesFile)
	if _, err := os.Stat(contactsPath); os.IsNotExist(err) {
		t.Fatal("contacts.toml was not created")
	}

	// Second call with same key - should be no-op
	err = EnsureContact(tmpDir, "cli", "alice", "Alice Updated")
	if err != nil {
		t.Fatalf("EnsureContact() second call error = %v", err)
	}

	// Verify only one entry exists
	cfg, err := loadIdentitiesFile(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(cfg.Contacts) != 1 {
		t.Errorf("len(contacts) = %d, want 1", len(cfg.Contacts))
	}

	// Verify first call's display name is preserved (not updated)
	if cfg.Contacts[0].DisplayName != "Alice" {
		t.Errorf("contact.DisplayName = %q, want %q", cfg.Contacts[0].DisplayName, "Alice")
	}

	// Third call with different interface - should add
	err = EnsureContact(tmpDir, "telegram", "12345", "Alice")
	if err != nil {
		t.Fatal(err)
	}

	cfg, err = loadIdentitiesFile(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(cfg.Contacts) != 2 {
		t.Errorf("len(contacts) after different interface = %d, want 2", len(cfg.Contacts))
	}
}

func TestLoadSave_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewJSONLStore(tmpDir)

	userID := "alice"
	sessionID := NewSessionID()

	// Save messages
	messages := []struct {
		role    string
		content string
	}{
		{"user", "Hello"},
		{"assistant", "Hi there!"},
		{"user", "How are you?"},
	}

	for _, msg := range messages {
		if err := store.Save(userID, sessionID, msg.role, msg.content); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
	}

	// Load with no limit
	loaded, err := store.Load(userID, sessionID, 0)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(loaded) != 0 {
		t.Errorf("Load(limit=0) returned %d messages, want 0", len(loaded))
	}

	// Load all messages
	loaded, err = store.Load(userID, sessionID, 10)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(loaded) != len(messages) {
		t.Fatalf("Load() returned %d messages, want %d", len(loaded), len(messages))
	}

	// Verify content
	for i, msg := range messages {
		if loaded[i].Role != msg.role {
			t.Errorf("Message[%d].Role = %q, want %q", i, loaded[i].Role, msg.role)
		}
		if loaded[i].Content != msg.content {
			t.Errorf("Message[%d].Content = %q, want %q", i, loaded[i].Content, msg.content)
		}
	}

	// Load with limit
	loaded, err = store.Load(userID, sessionID, 2)
	if err != nil {
		t.Fatalf("Load(limit=2) error = %v", err)
	}

	if len(loaded) != 2 {
		t.Errorf("Load(limit=2) returned %d messages, want 2", len(loaded))
	}

	// Should get last 2 messages
	if loaded[0].Content != "Hi there!" {
		t.Errorf("Last 2 messages[0] = %q, want %q", loaded[0].Content, "Hi there!")
	}
}

func TestLoadSave_NewPath(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewJSONLStore(tmpDir)

	// Verify path structure: .data/sessions/{userID}/{sessionID}.jsonl
	userID := "alice"
	sessionID := "20260614_120000_abc123"

	if err := store.Save(userID, sessionID, "user", "test"); err != nil {
		t.Fatal(err)
	}

	expectedPath := filepath.Join(tmpDir, ".data", "sessions", userID, sessionID+".jsonl")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("Session file not created at expected path: %s", expectedPath)
	}

	// Verify directory was created
	userDir := filepath.Join(tmpDir, ".data", "sessions", userID)
	if info, err := os.Stat(userDir); err != nil || !info.IsDir() {
		t.Errorf("User session directory not created: %s", userDir)
	}
}

func TestMigrateContact(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .data directory and contacts.toml
	dataDir := filepath.Join(tmpDir, ".data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}

	identitiesContent := `[[identity]]
id = "alice"
contacts = ["telegram:12345"]
`
	identitiesPath := filepath.Join(dataDir, "contacts.toml")
	if err := os.WriteFile(identitiesPath, []byte(identitiesContent), 0644); err != nil {
		t.Fatal(err)
	}

	sessionsDir := filepath.Join(tmpDir, ".data", "sessions")

	t.Run("fast path - no unlinked folder", func(t *testing.T) {
		// Should return immediately (no folder exists)
		err := MigrateContact(tmpDir, "telegram", "99999")
		if err != nil {
			t.Errorf("MigrateContact() error = %v, want nil", err)
		}
	})

	t.Run("migrate existing unlinked folder", func(t *testing.T) {
		// Create unlinked folder with session files
		unlinkedDir := filepath.Join(sessionsDir, "telegram-12345")
		if err := os.MkdirAll(unlinkedDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Add session file
		sessionFile := filepath.Join(unlinkedDir, "20260614_100000_abc123.jsonl")
		if err := os.WriteFile(sessionFile, []byte(`{"role":"user","content":"test"}`), 0644); err != nil {
			t.Fatal(err)
		}

		// Add .last_session
		lastSessionFile := filepath.Join(unlinkedDir, ".last_session")
		if err := os.WriteFile(lastSessionFile, []byte("telegram=20260614_100000_abc123\n"), 0644); err != nil {
			t.Fatal(err)
		}

		// Run migration
		err := MigrateContact(tmpDir, "telegram", "12345")
		if err != nil {
			t.Fatalf("MigrateContact() error = %v", err)
		}

		// Verify unlinked folder is gone
		if _, err := os.Stat(unlinkedDir); !os.IsNotExist(err) {
			t.Errorf("Unlinked folder still exists after migration: %s", unlinkedDir)
		}

		// Verify files moved to alice folder
		aliceDir := filepath.Join(sessionsDir, "alice")
		movedSession := filepath.Join(aliceDir, "20260614_100000_abc123.jsonl")
		if _, err := os.Stat(movedSession); os.IsNotExist(err) {
			t.Errorf("Session file not migrated to alice folder: %s", movedSession)
		}

		// Verify .last_session exists
		aliceLastSession := filepath.Join(aliceDir, ".last_session")
		if _, err := os.Stat(aliceLastSession); os.IsNotExist(err) {
			t.Errorf(".last_session not migrated to alice folder: %s", aliceLastSession)
		}
	})

	t.Run("idempotent - running again is no-op", func(t *testing.T) {
		// Run migration again - should be instant (no unlinked folder exists)
		err := MigrateContact(tmpDir, "telegram", "12345")
		if err != nil {
			t.Errorf("MigrateContact() second run error = %v", err)
		}
	})

	t.Run("no migration for truly unlinked contacts", func(t *testing.T) {
		// Create unlinked folder for contact NOT in any identity
		unlinkedDir := filepath.Join(sessionsDir, "telegram-99999")
		if err := os.MkdirAll(unlinkedDir, 0755); err != nil {
			t.Fatal(err)
		}

		sessionFile := filepath.Join(unlinkedDir, "20260614_110000_xyz789.jsonl")
		if err := os.WriteFile(sessionFile, []byte(`{"role":"user","content":"test"}`), 0644); err != nil {
			t.Fatal(err)
		}

		// Run migration - should not migrate (contact not in any identity)
		err := MigrateContact(tmpDir, "telegram", "99999")
		if err != nil {
			t.Errorf("MigrateContact() error = %v", err)
		}

		// Verify folder still exists (not migrated)
		if _, err := os.Stat(unlinkedDir); os.IsNotExist(err) {
			t.Errorf("Unlinked folder was removed but shouldn't be (contact not in identity)")
		}
	})
}
