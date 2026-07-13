package syscommands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DeprecatedLuar/agentctl/internal/session"
)

func TestNewSession_CreatesSessionFile(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")

	tmpDir := t.TempDir()

	configDir := filepath.Join(tmpDir, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	agentToml := `
[agent]
provider = "openrouter"
model = "openrouter/free"

[memory]
max_messages = 10
`
	if err := os.WriteFile(filepath.Join(configDir, "agent.toml"), []byte(agentToml), 0644); err != nil {
		t.Fatal(err)
	}

	store := session.NewJSONLStore(tmpDir)

	result, err := NewSession("alice", "cli", store, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Type != ResultTypeNewSession {
		t.Errorf("result type = %v, want ResultTypeNewSession", result.Type)
	}

	sessionID, err := store.GetLast("alice", "cli")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sessionID == "" {
		t.Fatal("GetLast returned empty session ID after /new")
	}

	// The session file must exist immediately, before any real chat turn.
	if !store.SessionExists("alice", sessionID) {
		t.Fatal("session file does not exist right after NewSession")
	}

	// Injecting into it (e.g. a scheduled delivery arriving right after
	// /new, before the user's first real message) must succeed.
	if err := session.InjectTurn(tmpDir, store, "alice", sessionID, "assistant", "hello"); err != nil {
		t.Fatalf("InjectTurn failed on freshly-/new'd session: %v", err)
	}
}
