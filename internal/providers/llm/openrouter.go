package llm

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/DeprecatedLuar/agentctl/internal/config"
	"github.com/joho/godotenv"
)

const (
	// OpenRouter configuration
	openrouterEnvFile  = ".env"
	openrouterAPIKey   = "OPENROUTER_API_KEY"
	openrouterBaseURL  = "https://openrouter.ai/api/v1"
	openrouterProvider = "openrouter"
)

// NewOpenRouterProvider creates an OpenRouter provider
func NewOpenRouterProvider(cfg *config.AgentConfig, agentFolder, userFolder string, recordExchanges bool, logger *slog.Logger) (Provider, error) {
	envPath := filepath.Join(agentFolder, openrouterEnvFile)
	_ = godotenv.Load(envPath)

	apiKey := os.Getenv(openrouterAPIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("%s not found in %s or environment", openrouterAPIKey, openrouterEnvFile)
	}

	return &baseProvider{
		client:       buildClient(apiKey, openrouterBaseURL),
		model:        cfg.Agent.Model,
		providerName: openrouterProvider,
		logger:       logger,
		userFolder:   userFolder,
		debugEnabled: recordExchanges && cfg.Agent.IsDebugMode(),

		// OpenRouter's Chat Completions extension is the only path in this
		// codebase that can round-trip reasoning_details (see provider.go).
		supportsReasoning: true,
		reasoningEnabled:  cfg.Advanced.ReasoningEnabled(),
		reasoningEffort:   cfg.Advanced.ReasoningEffort,
	}, nil
}
