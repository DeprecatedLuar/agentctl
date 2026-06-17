package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInjectTurn(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Create session directories
	sessionsDir := filepath.Join(tmpDir, ".data", "sessions", "alice")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create session store
	store := NewJSONLStore(tmpDir)

	// Create a session by saving a message
	sessionID := "20250614_abc123"
	if err := store.Save("alice", sessionID, "user", "hello"); err != nil {
		t.Fatal(err)
	}

	// Test successful injection
	err := InjectTurn(store, "alice", sessionID, "assistant", "hi there")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify the turn was saved
	messages, err := store.Load("alice", sessionID, 10)
	if err != nil {
		t.Fatal(err)
	}

	if len(messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(messages))
	}

	if messages[1].Role != "assistant" {
		t.Errorf("role = %q, want 'assistant'", messages[1].Role)
	}

	if messages[1].Content != "hi there" {
		t.Errorf("content = %q, want 'hi there'", messages[1].Content)
	}

	// Test injection into non-existent session
	err = InjectTurn(store, "alice", "nonexistent", "user", "test")
	if err == nil {
		t.Error("expected error for non-existent session, got nil")
	}

	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error message should mention session doesn't exist, got: %v", err)
	}
}
