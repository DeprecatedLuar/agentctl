package llm

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/DeprecatedLuar/agentctl/internal/config"
	"github.com/joho/godotenv"
)

const (
	// Generic provider configuration
	genericEnvFile    = ".env"
	genericAPIKey     = "OPENAI_API_KEY"
	genericProviderName = "generic"
)

// NewGenericProvider creates a provider for OpenAI-compatible endpoints
func NewGenericProvider(cfg *config.AgentConfig, agentFolder, userFolder string, recordExchanges bool, logger *slog.Logger) (Provider, error) {
	// Load .env from agent folder (optional)
	envPath := filepath.Join(agentFolder, genericEnvFile)
	_ = godotenv.Load(envPath)

	// API key is optional for custom endpoints (local servers may not need auth)
	apiKey := os.Getenv(genericAPIKey)

	return &baseProvider{
		client:       buildClient(apiKey, cfg.Agent.Provider),
		model:        cfg.Agent.Model,
		providerName: genericProviderName,
		logger:       logger,
		userFolder:   userFolder,
		debugEnabled: recordExchanges && cfg.Agent.IsDebugMode(),
	}, nil
}
