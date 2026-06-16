package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJSONLStore_SaveLoad(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	store := NewJSONLStore(tmpDir)

	userID := "testuser"
	sessionID := "20260615_120000_abc123"

	// Save messages
	if err := store.Save(userID, sessionID, "user", "hello"); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := store.Save(userID, sessionID, "assistant", "hi there"); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load messages
	messages, err := store.Load(userID, sessionID, 10)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Should have 2 messages (metadata line is skipped)
	if len(messages) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(messages))
	}

	if messages[0].Role != "user" || messages[0].Content != "hello" {
		t.Errorf("First message mismatch: %+v", messages[0])
	}

	if messages[1].Role != "assistant" || messages[1].Content != "hi there" {
		t.Errorf("Second message mismatch: %+v", messages[1])
	}
}

func TestJSONLStore_MetadataFirstLine(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	store := NewJSONLStore(tmpDir)

	userID := "testuser"
	sessionID := "20260615_120000_abc123"

	// Save first message (creates metadata line)
	if err := store.Save(userID, sessionID, "user", "hello"); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Read file directly to check first line
	path := store.sessionFilePath(userID, sessionID)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read session file: %v", err)
	}

	// Check that file starts with metadata line
	content := string(data)
	if len(content) == 0 {
		t.Fatal("Session file is empty")
	}

	// Should contain type=meta in first line
	if !containsString(content, `"type":"meta"`) {
		t.Error("First line should contain metadata")
	}
}

func TestJSONLStore_GetSetMeta(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewJSONLStore(tmpDir)

	userID := "alice"
	sessionID := "20260615_120000_abc123"

	// Create a session
	if err := store.Save(userID, sessionID, "user", "test message"); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Get initial metadata
	meta, err := store.GetMeta(userID, sessionID)
	if err != nil {
		t.Fatalf("GetMeta failed: %v", err)
	}

	if meta.Title != "" {
		t.Errorf("Initial title should be empty, got: %s", meta.Title)
	}

	if meta.UserID != userID {
		t.Errorf("UserID mismatch: expected %s, got %s", userID, meta.UserID)
	}

	// Update metadata
	meta.Title = "Test Session"
	meta.Interface = "cli"

	if err := store.SetMeta(userID, sessionID, meta); err != nil {
		t.Fatalf("SetMeta failed: %v", err)
	}

	// Verify update
	updatedMeta, err := store.GetMeta(userID, sessionID)
	if err != nil {
		t.Fatalf("GetMeta after update failed: %v", err)
	}

	if updatedMeta.Title != "Test Session" {
		t.Errorf("Title not updated: %s", updatedMeta.Title)
	}

	if updatedMeta.Interface != "cli" {
		t.Errorf("Interface not updated: %s", updatedMeta.Interface)
	}

	// Verify messages still intact after SetMeta
	messages, err := store.Load(userID, sessionID, 10)
	if err != nil {
		t.Fatalf("Load after SetMeta failed: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message after SetMeta, got %d", len(messages))
	}

	if messages[0].Content != "test message" {
		t.Errorf("Message corrupted after SetMeta: %s", messages[0].Content)
	}
}

func TestJSONLStore_LoadSkipsMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewJSONLStore(tmpDir)

	userID := "testuser"
	sessionID := "20260615_120000_abc123"

	// Create session with multiple messages
	if err := store.Save(userID, sessionID, "user", "msg1"); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := store.Save(userID, sessionID, "assistant", "msg2"); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := store.Save(userID, sessionID, "user", "msg3"); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load all messages
	messages, err := store.Load(userID, sessionID, 10)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Should return only conversation messages, not metadata
	if len(messages) != 3 {
		t.Fatalf("Expected 3 messages, got %d", len(messages))
	}

	// Verify no metadata line in messages
	for _, msg := range messages {
		if msg.Role == "meta" {
			t.Error("Metadata line should not be included in Load results")
		}
	}
}

func TestJSONLStore_GetSetLast(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewJSONLStore(tmpDir)

	userID := "alice"
	iface := "cli"

	// Initially empty
	last, err := store.GetLast(userID, iface)
	if err != nil {
		t.Fatalf("GetLast failed: %v", err)
	}
	if last != "" {
		t.Errorf("Expected empty last session, got: %s", last)
	}

	// Set last session
	sessionID := "20260615_120000_abc123"
	if err := store.SetLast(userID, iface, sessionID); err != nil {
		t.Fatalf("SetLast failed: %v", err)
	}

	// Retrieve last session
	last, err = store.GetLast(userID, iface)
	if err != nil {
		t.Fatalf("GetLast after SetLast failed: %v", err)
	}
	if last != sessionID {
		t.Errorf("Expected %s, got %s", sessionID, last)
	}

	// Update to new session
	newSessionID := "20260615_130000_def456"
	if err := store.SetLast(userID, iface, newSessionID); err != nil {
		t.Fatalf("SetLast update failed: %v", err)
	}

	last, err = store.GetLast(userID, iface)
	if err != nil {
		t.Fatalf("GetLast after update failed: %v", err)
	}
	if last != newSessionID {
		t.Errorf("Expected %s, got %s", newSessionID, last)
	}
}

func TestJSONLStore_SetLastAtomic(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewJSONLStore(tmpDir)

	userID := "alice"

	// Set multiple interfaces
	if err := store.SetLast(userID, "cli", "session1"); err != nil {
		t.Fatalf("SetLast failed: %v", err)
	}
	if err := store.SetLast(userID, "telegram", "session2"); err != nil {
		t.Fatalf("SetLast failed: %v", err)
	}

	// Update one interface
	if err := store.SetLast(userID, "cli", "session3"); err != nil {
		t.Fatalf("SetLast update failed: %v", err)
	}

	// Verify both interfaces preserved
	cliLast, _ := store.GetLast(userID, "cli")
	if cliLast != "session3" {
		t.Errorf("CLI last session should be session3, got: %s", cliLast)
	}

	tgLast, _ := store.GetLast(userID, "telegram")
	if tgLast != "session2" {
		t.Errorf("Telegram last session should be session2, got: %s", tgLast)
	}
}

func TestJSONLStore_ListSessions(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewJSONLStore(tmpDir)

	userID := "alice"

	// Create multiple sessions
	session1 := "20260615_120000_abc123"
	session2 := "20260615_130000_def456"
	session3 := "20260615_140000_ghi789"

	if err := store.Save(userID, session1, "user", "msg1"); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := store.Save(userID, session2, "user", "msg2"); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := store.Save(userID, session3, "user", "msg3"); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Update metadata for one session
	meta, _ := store.GetMeta(userID, session1)
	meta.Title = "First Session"
	store.SetMeta(userID, session1, meta)

	// List sessions
	sessions, err := store.ListSessions(userID)
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}

	if len(sessions) != 3 {
		t.Fatalf("Expected 3 sessions, got %d", len(sessions))
	}

	// Find session with title
	foundTitle := false
	for _, s := range sessions {
		if s.Title == "First Session" {
			foundTitle = true
			break
		}
	}
	if !foundTitle {
		t.Error("Session with title 'First Session' not found in ListSessions")
	}
}

func TestJSONLStore_NonExistentSession(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewJSONLStore(tmpDir)

	// Load non-existent session should return empty, not error
	messages, err := store.Load("alice", "nonexistent", 10)
	if err != nil {
		t.Fatalf("Load non-existent session should not error: %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("Expected empty messages, got %d", len(messages))
	}

	// GetMeta non-existent session should return empty metadata
	meta, err := store.GetMeta("alice", "nonexistent")
	if err != nil {
		t.Fatalf("GetMeta non-existent session should not error: %v", err)
	}
	if meta.Title != "" {
		t.Errorf("Expected empty metadata, got: %+v", meta)
	}
}

func TestJSONLStore_LimitMessages(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewJSONLStore(tmpDir)

	userID := "testuser"
	sessionID := "20260615_120000_abc123"

	// Save 5 messages
	for i := 1; i <= 5; i++ {
		role := "user"
		if i%2 == 0 {
			role = "assistant"
		}
		if err := store.Save(userID, sessionID, role, "message"); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}

	// Load with limit
	messages, err := store.Load(userID, sessionID, 3)
	if err != nil {
		t.Fatalf("Load with limit failed: %v", err)
	}

	// Should return last 3 messages
	if len(messages) != 3 {
		t.Fatalf("Expected 3 messages with limit=3, got %d", len(messages))
	}

	// Load with limit=0 should return empty
	messages, err = store.Load(userID, sessionID, 0)
	if err != nil {
		t.Fatalf("Load with limit=0 failed: %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("Expected 0 messages with limit=0, got %d", len(messages))
	}
}

func TestJSONLStore_EnsureContactDelegation(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewJSONLStore(tmpDir)

	// This should not error and should create contacts file
	err := store.EnsureContact("cli", "alice", "Alice")
	if err != nil {
		t.Fatalf("EnsureContact failed: %v", err)
	}

	// Verify contacts file was created
	contactsPath := filepath.Join(tmpDir, contactsFile)
	if _, err := os.Stat(contactsPath); os.IsNotExist(err) {
		t.Error("Contacts file was not created")
	}
}

func TestJSONLStore_ResolveUserDelegation(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewJSONLStore(tmpDir)

	// Without identities file, should return unlinked folder name
	userID, err := store.ResolveUser("cli", "alice")
	if err != nil {
		t.Fatalf("ResolveUser failed: %v", err)
	}

	expected := UnlinkedFolderName("cli", "alice")
	if userID != expected {
		t.Errorf("Expected unlinked folder name %s, got %s", expected, userID)
	}
}

// Helper function
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && contains(s, substr))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
