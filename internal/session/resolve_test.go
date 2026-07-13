package session

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveChannel(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Create identities.toml
	identitiesPath := filepath.Join(tmpDir, ".data")
	if err := os.MkdirAll(identitiesPath, 0755); err != nil {
		t.Fatal(err)
	}

	identitiesContent := `
[[identity]]
id = "alice"
contacts = ["cli:alice", "telegram:123456789"]

[[identity]]
id = "bob"
contacts = ["cli:bob", "telegram:987654321"]
`
	if err := os.WriteFile(filepath.Join(identitiesPath, "contacts.toml"), []byte(identitiesContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create session directories
	sessionsDir := filepath.Join(tmpDir, ".data", "sessions")
	if err := os.MkdirAll(filepath.Join(sessionsDir, "alice"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sessionsDir, "bob"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create .last_session file for alice
	lastSessionPath := filepath.Join(sessionsDir, "alice", ".last_session")
	if err := os.WriteFile(lastSessionPath, []byte("telegram=20250614_abc123\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create session store
	store := NewJSONLStore(tmpDir)

	tests := []struct {
		name           string
		channelStr     string
		currentUserID  string
		wantUserID     string
		wantPlatformID string
		wantError      bool
	}{
		{
			name:           "bare interface - telegram",
			channelStr:     "telegram",
			currentUserID:  "alice",
			wantUserID:     "alice",
			wantPlatformID: "123456789",
			wantError:      false,
		},
		{
			name:           "explicit user@interface",
			channelStr:     "bob@telegram",
			currentUserID:  "alice",
			wantUserID:     "bob",
			wantPlatformID: "987654321",
			wantError:      false,
		},
		{
			name:           "cli interface",
			channelStr:     "alice@cli",
			currentUserID:  "alice",
			wantUserID:     "alice",
			wantPlatformID: "alice",
			wantError:      false,
		},
		{
			name:           "user not found",
			channelStr:     "charlie@telegram",
			currentUserID:  "alice",
			wantUserID:     "",
			wantPlatformID: "",
			wantError:      true,
		},
		{
			name:           "interface not found for user",
			channelStr:     "alice@discord",
			currentUserID:  "alice",
			wantUserID:     "",
			wantPlatformID: "",
			wantError:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID, platformID, sessionID, err := ResolveChannel(store, tmpDir, tt.channelStr, tt.currentUserID)

			if tt.wantError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if userID != tt.wantUserID {
				t.Errorf("userID = %q, want %q", userID, tt.wantUserID)
			}

			if platformID != tt.wantPlatformID {
				t.Errorf("platformID = %q, want %q", platformID, tt.wantPlatformID)
			}

			// Session ID should be either existing or newly created
			if sessionID == "" {
				t.Errorf("sessionID is empty")
			}

			// For alice telegram, should return existing session
			if tt.channelStr == "telegram" && tt.currentUserID == "alice" {
				if sessionID != "20250614_abc123" {
					t.Errorf("sessionID = %q, want existing session 20250614_abc123", sessionID)
				}
			}
		})
	}
}

func TestMintSession(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewJSONLStore(tmpDir)

	sessionID, err := MintSession(store, "alice", "telegram")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sessionID == "" {
		t.Fatal("sessionID is empty")
	}

	// The file must exist immediately, before any Save call.
	if !store.SessionExists("alice", sessionID) {
		t.Fatal("session file does not exist right after MintSession")
	}

	// GetLast must point at the minted session.
	last, err := store.GetLast("alice", "telegram")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if last != sessionID {
		t.Errorf("GetLast = %q, want %q", last, sessionID)
	}

	// InjectTurn must succeed with no prior Save - this is the exact
	// failure mode from the live bug (turn silently lost because the
	// session file didn't exist yet).
	if err := InjectTurn(tmpDir, store, "alice", sessionID, "assistant", "hello"); err != nil {
		t.Fatalf("InjectTurn failed on freshly minted session: %v", err)
	}

	messages, err := store.Load("alice", sessionID, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(messages) != 1 || messages[0].Content != "hello" {
		t.Errorf("messages = %v, want single injected turn", messages)
	}
}

func TestResolveChannel_ColdStartCreatesSessionFile(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	identitiesPath := filepath.Join(tmpDir, ".data")
	if err := os.MkdirAll(identitiesPath, 0755); err != nil {
		t.Fatal(err)
	}

	identitiesContent := `
[[identity]]
id = "bob"
contacts = ["cli:bob", "telegram:987654321"]
`
	if err := os.WriteFile(filepath.Join(identitiesPath, "contacts.toml"), []byte(identitiesContent), 0644); err != nil {
		t.Fatal(err)
	}

	store := NewJSONLStore(tmpDir)

	// bob has no prior session at all - this is the cold-start delivery case.
	userID, _, sessionID, err := ResolveChannel(store, tmpDir, "telegram", "bob")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !store.SessionExists(userID, sessionID) {
		t.Fatal("session file does not exist right after ResolveChannel's cold-start mint")
	}

	// InjectTurn must succeed immediately, matching real deliverToChannels usage.
	if err := InjectTurn(tmpDir, store, userID, sessionID, "assistant", "reminder"); err != nil {
		t.Fatalf("InjectTurn failed on cold-start resolved session: %v", err)
	}
}

func TestLookupPlatformID(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Create identities.toml
	identitiesPath := filepath.Join(tmpDir, ".data")
	if err := os.MkdirAll(identitiesPath, 0755); err != nil {
		t.Fatal(err)
	}

	identitiesContent := `
[[identity]]
id = "alice"
contacts = ["cli:alice", "telegram:123456789", "telegram:111111111"]

[[identity]]
id = "bob"
contacts = ["cli:bob"]
`
	if err := os.WriteFile(filepath.Join(identitiesPath, "contacts.toml"), []byte(identitiesContent), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name           string
		userID         string
		gateway        string
		wantPlatformID string
		wantError      bool
	}{
		{
			name:           "alice telegram - first match wins",
			userID:         "alice",
			gateway:        "telegram",
			wantPlatformID: "123456789",
			wantError:      false,
		},
		{
			name:           "alice cli",
			userID:         "alice",
			gateway:        "cli",
			wantPlatformID: "alice",
			wantError:      false,
		},
		{
			name:           "bob cli",
			userID:         "bob",
			gateway:        "cli",
			wantPlatformID: "bob",
			wantError:      false,
		},
		{
			name:           "bob telegram - not found",
			userID:         "bob",
			gateway:        "telegram",
			wantPlatformID: "",
			wantError:      true,
		},
		{
			name:           "charlie - user not found",
			userID:         "charlie",
			gateway:        "cli",
			wantPlatformID: "",
			wantError:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			platformID, err := LookupPlatformID(tmpDir, tt.userID, tt.gateway)

			if tt.wantError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if platformID != tt.wantPlatformID {
				t.Errorf("platformID = %q, want %q", platformID, tt.wantPlatformID)
			}
		})
	}
}

func TestWarnDuplicateContacts(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Create identities.toml with duplicates
	identitiesPath := filepath.Join(tmpDir, ".data")
	if err := os.MkdirAll(identitiesPath, 0755); err != nil {
		t.Fatal(err)
	}

	identitiesContent := `
[[identity]]
id = "alice"
contacts = ["cli:alice", "telegram:123456789", "telegram:111111111"]

[[identity]]
id = "bob"
contacts = ["cli:bob", "cli:robert"]
`
	if err := os.WriteFile(filepath.Join(identitiesPath, "contacts.toml"), []byte(identitiesContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a simple logger that captures warnings
	var warnings []string
	captureLogger := newTestLogger(func(msg string) {
		if strings.Contains(msg, "multiple contacts") {
			warnings = append(warnings, msg)
		}
	})

	// Call WarnDuplicateContacts
	err := WarnDuplicateContacts(tmpDir, captureLogger)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Check that warnings were logged for both identities
	if len(warnings) != 2 {
		t.Errorf("expected 2 warnings, got %d: %v", len(warnings), warnings)
	}
}

// newTestLogger creates a logger that calls a function on each Warn call
func newTestLogger(onWarn func(string)) *slog.Logger {
	return slog.New(&testHandler{onWarn: onWarn})
}

type testHandler struct {
	onWarn func(string)
}

func (h *testHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

func (h *testHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level == slog.LevelWarn && h.onWarn != nil {
		h.onWarn(r.Message)
	}
	return nil
}

func (h *testHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *testHandler) WithGroup(name string) slog.Handler {
	return h
}
