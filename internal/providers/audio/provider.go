package audio

import (
	"fmt"
	"strings"

	"github.com/DeprecatedLuar/agentctl/internal/config"
)

const (
	// Provider names
	ProviderWhisperCLI = "whisper-cli"
	ProviderWhisper    = "whisper"
)

// Transcriber handles audio transcription
type Transcriber interface {
	// Transcribe converts audio bytes to text
	// filename is used for extension detection (e.g., "audio.mp3")
	Transcribe(audioData []byte, filename string) (string, error)
}

// New creates a transcriber based on the config
func New(cfg *config.AudioConfig, agentFolder string) (Transcriber, error) {
	if cfg == nil {
		return nil, fmt.Errorf("audio config is nil")
	}

	// Check if provider is a custom endpoint (http/https URL)
	if strings.HasPrefix(cfg.Provider, "http://") || strings.HasPrefix(cfg.Provider, "https://") {
		return NewWhisperAPI(cfg, agentFolder)
	}

	// Named providers
	switch cfg.Provider {
	case ProviderWhisperCLI:
		return NewWhisperCLI(cfg)
	case ProviderWhisper:
		return NewWhisperAPI(cfg, agentFolder)
	default:
		return nil, fmt.Errorf("unsupported audio provider: %s (must be 'whisper-cli', 'whisper', or an http/https URL)", cfg.Provider)
	}
}
