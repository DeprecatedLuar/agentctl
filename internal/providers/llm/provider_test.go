package llm

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/DeprecatedLuar/agentctl/internal/config"
)

// TestWarnUnsupportedReasoning_OnlyOncePerProvider verifies the warning fires
// the first time reasoning_carryover is requested on a provider that can't
// honor it, and stays silent on subsequent calls for that same provider name
// (a long-running `serve` daemon rebuilds providers on every request under
// hot-reload, so without dedup this would repeat on every message).
func TestWarnUnsupportedReasoning_OnlyOncePerProvider(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := &config.AgentConfig{Advanced: config.AdvancedConfig{ReasoningCarryover: "tools"}}

	providerName := "test-provider-" + t.Name()

	warnUnsupportedReasoning(logger, providerName, cfg)
	warnUnsupportedReasoning(logger, providerName, cfg)
	warnUnsupportedReasoning(logger, providerName, cfg)

	out := buf.String()
	count := strings.Count(out, "does not support reasoning preservation")
	if count != 1 {
		t.Errorf("expected exactly 1 warning log, got %d: %q", count, out)
	}
}

// TestWarnUnsupportedReasoning_SilentWhenDisabled verifies no warning fires
// when reasoning_carryover is off (mode "none") — nothing to warn about.
func TestWarnUnsupportedReasoning_SilentWhenDisabled(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := &config.AgentConfig{Advanced: config.AdvancedConfig{ReasoningCarryover: config.ReasoningNone}}

	warnUnsupportedReasoning(logger, "test-provider-"+t.Name(), cfg)

	if buf.Len() != 0 {
		t.Errorf("expected no log output, got: %q", buf.String())
	}
}
