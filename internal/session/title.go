package session

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/DeprecatedLuar/agentctl/internal/config"
	"github.com/DeprecatedLuar/agentctl/internal/providers/llm"
)

const titlePrompt = `Generate a short session title (max 5 words) based on this conversation.
Respond with only the title, no punctuation.

User: %s
Assistant: %s`

// GenerateTitle calls the configured provider with a fixed prompt using the
// provided user + assistant messages. Updates meta via SetMeta.
// No-op if meta.Title is already set.
func GenerateTitle(store SessionStore, cfg *config.AgentConfig, agentFolder, userID, sessionID, userMsg, assistantMsg string, logger *slog.Logger) error {
	if logger != nil {
		logger.Debug("GenerateTitle called", "session", sessionID)
	}

	// Check if title already set (guard against re-generation)
	meta, err := store.GetMeta(userID, sessionID)
	if err != nil {
		if logger != nil {
			logger.Debug("failed to get metadata", "error", err)
		}
		return fmt.Errorf("failed to get metadata: %w", err)
	}

	if meta.Title != "" {
		if logger != nil {
			logger.Debug("title already set, skipping", "title", meta.Title)
		}
		return nil
	}

	// Validate messages are non-empty
	if userMsg == "" || assistantMsg == "" {
		if logger != nil {
			logger.Debug("empty messages provided", "user_empty", userMsg == "", "assistant_empty", assistantMsg == "")
		}
		return nil
	}

	// Create provider using same logic as agent.Run(), but don't record this
	// exchange to last-call.json — it would clobber the real conversation's debug record.
	prov, err := llm.NewProvider(cfg, agentFolder, UserFolder(agentFolder, userID), false, logger)
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	// Build prompt with first exchange
	prompt := fmt.Sprintf(titlePrompt, userMsg, assistantMsg)

	// Call provider (no tools needed for title generation)
	response, _, err := prov.SendMessages([]llm.Message{
		{Role: "user", Content: prompt},
	}, nil)

	if err != nil {
		return fmt.Errorf("provider error: %w", err)
	}

	// Clean up response (trim whitespace and punctuation)
	title := strings.TrimSpace(response)
	title = strings.Trim(title, ".,!?;:")

	// Update metadata. Locked (unlike the LLM call above) since this is the
	// only part that touches shared session state - by the time this runs,
	// the turn that triggered title generation has already released its own
	// lock (title's LLM call happens first, then this), so no self-deadlock.
	unlock, err := LockSession(agentFolder, userID, sessionID, logger)
	if err != nil {
		return fmt.Errorf("failed to lock session: %w", err)
	}
	meta.Title = title
	err = store.SetMeta(userID, sessionID, meta)
	unlock()
	if err != nil {
		return fmt.Errorf("failed to set metadata: %w", err)
	}

	if logger != nil {
		logger.Info("title generated", "session", sessionID, "title", title)
	}

	return nil
}
