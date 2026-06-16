package session

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/DeprecatedLuar/agentctl/internal/config"
	"github.com/DeprecatedLuar/agentctl/internal/registry"
	"github.com/joho/godotenv"
)

// ensureTestCredentials tries to load API credentials for testing
// 1. Checks if OPENROUTER_API_KEY already in env
// 2. Falls back to loading from a registered agent's .env
// Returns true if credentials available, false otherwise
func ensureTestCredentials(t *testing.T) bool {
	// Check if already set
	if os.Getenv("OPENROUTER_API_KEY") != "" {
		return true
	}

	// Try to find a registered agent
	agents, err := registry.List()
	if err != nil || len(agents) == 0 {
		return false
	}

	// Try to load .env from first agent
	envPath := filepath.Join(agents[0], ".env")
	if err := godotenv.Load(envPath); err != nil {
		return false
	}

	// Check if key was loaded
	if os.Getenv("OPENROUTER_API_KEY") != "" {
		t.Logf("Using credentials from agent: %s", agents[0])
		return true
	}

	return false
}

func TestGenerateTitle(t *testing.T) {
	// Ensure we have API credentials (env var or registry lookup)
	if !ensureTestCredentials(t) {
		t.Skip("skipping test - no OPENROUTER_API_KEY in env and no registered agents with credentials")
	}

	// Create temporary test agent folder
	tmpDir := t.TempDir()

	// Create minimal agent config
	configDir := filepath.Join(tmpDir, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Write minimal agent.toml with a real free model
	configContent := `provider = "openrouter"
model = "openrouter/free"

[memory]
max_messages = 10
`
	configPath := filepath.Join(configDir, "agent.toml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Create session store
	store := NewJSONLStore(tmpDir)

	// Test messages
	userID := "test-user"
	sessionID := "20260616_120000_abc123"
	userMsg := "Hello, what is 2+2?"
	assistantMsg := "2 + 2 = 4"

	if err := store.Save(userID, sessionID, "user", userMsg); err != nil {
		t.Fatalf("failed to save user message: %v", err)
	}
	if err := store.Save(userID, sessionID, "assistant", assistantMsg); err != nil {
		t.Fatalf("failed to save assistant message: %v", err)
	}

	// Load agent config
	cfg, _ := config.LoadAgent(tmpDir)
	if cfg == nil {
		t.Fatal("agent config failed to load")
	}

	// Create logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	t.Run("generates title on first turn", func(t *testing.T) {
		// Generate title
		if err := GenerateTitle(store, cfg, tmpDir, userID, sessionID, userMsg, assistantMsg, logger); err != nil {
			t.Fatalf("GenerateTitle failed: %v", err)
		}

		// Verify title was set
		meta, err := store.GetMeta(userID, sessionID)
		if err != nil {
			t.Fatalf("failed to get metadata: %v", err)
		}

		if meta.Title == "" {
			t.Error("expected title to be set, got empty string")
		}

		t.Logf("Generated title: %q", meta.Title)
	})

	t.Run("no-op if title already set", func(t *testing.T) {
		// Get current title
		meta, _ := store.GetMeta(userID, sessionID)
		originalTitle := meta.Title

		// Call GenerateTitle again
		if err := GenerateTitle(store, cfg, tmpDir, userID, sessionID, userMsg, assistantMsg, logger); err != nil {
			t.Fatalf("GenerateTitle failed: %v", err)
		}

		// Verify title unchanged
		meta, _ = store.GetMeta(userID, sessionID)
		if meta.Title != originalTitle {
			t.Errorf("expected title to remain %q, got %q", originalTitle, meta.Title)
		}
	})

	t.Run("no-op if empty messages", func(t *testing.T) {
		// Create new session
		newSessionID := "20260616_120001_def456"
		if err := store.Save(userID, newSessionID, "user", "Hi"); err != nil {
			t.Fatalf("failed to save message: %v", err)
		}

		// Try to generate title with empty assistant message
		if err := GenerateTitle(store, cfg, tmpDir, userID, newSessionID, "Hi", "", logger); err != nil {
			t.Fatalf("GenerateTitle failed: %v", err)
		}

		// Verify no title was set
		meta, _ := store.GetMeta(userID, newSessionID)
		if meta.Title != "" {
			t.Errorf("expected empty title for empty messages, got %q", meta.Title)
		}
	})
}
