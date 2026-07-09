package audio

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/DeprecatedLuar/agentctl/internal/config"
	"github.com/joho/godotenv"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const (
	whisperEnvFile      = ".env"
	whisperAPIKey       = "OPENAI_API_KEY"
	whisperDefaultURL   = "https://api.openai.com/v1"
	whisperDefaultModel = "whisper-1"
	defaultAudioExt     = ".mp3"
)

// writeTempAudioFile writes audioData to a new temp file, defaulting the
// extension to defaultAudioExt when ext is empty. It returns the file path
// and a cleanup function that removes the file; callers must invoke cleanup
// (typically via defer) once done with the file.
func writeTempAudioFile(audioData []byte, ext string) (path string, cleanup func(), err error) {
	if ext == "" {
		ext = defaultAudioExt
	}

	tmpFile, err := os.CreateTemp("", "whisper-*"+ext)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	cleanup = func() { os.Remove(tmpPath) }

	if _, err := tmpFile.Write(audioData); err != nil {
		tmpFile.Close()
		cleanup()
		return "", nil, fmt.Errorf("failed to write audio data: %w", err)
	}
	tmpFile.Close()

	return tmpPath, cleanup, nil
}

// WhisperAPI implements Transcriber using OpenAI-compatible API
type WhisperAPI struct {
	client   openai.Client
	model    string
	endpoint string
}

// NewWhisperAPI creates an API-based whisper transcriber
func NewWhisperAPI(cfg *config.AudioConfig, agentFolder string) (*WhisperAPI, error) {
	// Load .env from agent folder (optional)
	envPath := filepath.Join(agentFolder, whisperEnvFile)
	_ = godotenv.Load(envPath)

	// Determine base URL
	var baseURL string
	if strings.HasPrefix(cfg.Provider, "http://") || strings.HasPrefix(cfg.Provider, "https://") {
		// Custom endpoint
		baseURL = cfg.Provider
	} else if cfg.Provider == ProviderWhisper {
		// Named "whisper" provider -> OpenAI endpoint
		baseURL = whisperDefaultURL
	} else {
		return nil, fmt.Errorf("invalid whisper API provider: %s", cfg.Provider)
	}

	// API key (required for OpenAI, optional for custom endpoints)
	apiKey := os.Getenv(whisperAPIKey)
	if cfg.Provider == ProviderWhisper && apiKey == "" {
		return nil, fmt.Errorf("%s not found in %s or environment", whisperAPIKey, whisperEnvFile)
	}

	// Build client options
	opts := []option.RequestOption{
		option.WithBaseURL(baseURL),
	}
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}

	client := openai.NewClient(opts...)

	model := cfg.Model
	if model == "" {
		model = whisperDefaultModel
	}

	return &WhisperAPI{
		client:   client,
		model:    model,
		endpoint: baseURL,
	}, nil
}

// Transcribe converts audio to text using Whisper API
func (w *WhisperAPI) Transcribe(audioData []byte, filename string) (string, error) {
	// Create temp file for audio (OpenAI SDK needs a file)
	ext := filepath.Ext(filename)
	tmpPath, cleanup, err := writeTempAudioFile(audioData, ext)
	if err != nil {
		return "", err
	}
	defer cleanup()

	// Call API
	ctx := context.Background()

	// Open the temp file for reading
	file, err := os.Open(tmpPath)
	if err != nil {
		return "", fmt.Errorf("failed to open temp file: %w", err)
	}
	defer file.Close()

	// Make transcription request
	resp, err := w.client.Audio.Transcriptions.New(ctx, openai.AudioTranscriptionNewParams{
		File:  io.Reader(file),
		Model: openai.AudioModel(w.model),
	})
	if err != nil {
		return "", fmt.Errorf("transcription API call failed: %w", err)
	}

	return resp.Text, nil
}
