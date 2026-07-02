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
	// OpenAI configuration
	openaiEnvFile  = ".env"
	openaiAPIKey   = "OPENAI_API_KEY"
	openaiBaseURL  = "https://api.openai.com/v1"
	openaiProvider = "openai"
)

// NewOpenAIProvider creates an OpenAI provider
func NewOpenAIProvider(cfg *config.AgentConfig, agentFolder, userFolder string, recordExchanges bool, logger *slog.Logger) (Provider, error) {
	envPath := filepath.Join(agentFolder, openaiEnvFile)
	_ = godotenv.Load(envPath)

	apiKey := os.Getenv(openaiAPIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("%s not found in %s or environment", openaiAPIKey, openaiEnvFile)
	}

	return &baseProvider{
		client:       buildClient(apiKey, openaiBaseURL),
		model:        cfg.Agent.Model,
		providerName: openaiProvider,
		logger:       logger,
		userFolder:   userFolder,
		debugEnabled: recordExchanges && cfg.Agent.IsDebugMode(),
	}, nil
}
