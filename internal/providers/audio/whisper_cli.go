package audio

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/DeprecatedLuar/agentctl/internal/config"
)

// WhisperCLI implements Transcriber using the whisper CLI tool
type WhisperCLI struct {
	model string
}

// NewWhisperCLI creates a CLI-based whisper transcriber
func NewWhisperCLI(cfg *config.AudioConfig) (*WhisperCLI, error) {
	// Check if whisper-cli is installed
	if _, err := exec.LookPath("whisper-cli"); err != nil {
		return nil, fmt.Errorf("whisper-cli not found in PATH")
	}

	model := cfg.Model
	if model == "" {
		model = "base" // Default model
	}

	return &WhisperCLI{
		model: model,
	}, nil
}

// Transcribe converts audio to text using whisper CLI
func (w *WhisperCLI) Transcribe(audioData []byte, filename string) (string, error) {
	// Create temp file for audio
	ext := filepath.Ext(filename)
	if ext == "" {
		ext = ".mp3" // Default
	}

	tmpFile, err := os.CreateTemp("", "whisper-*"+ext)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath) // Clean up

	// Write audio data
	if _, err := tmpFile.Write(audioData); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("failed to write audio data: %w", err)
	}
	tmpFile.Close()

	// Convert to WAV if not already WAV (whisper-cli requires WAV/supported formats)
	audioPath := tmpPath
	if ext != ".wav" {
		wavPath := tmpPath + ".wav"
		defer os.Remove(wavPath)

		// Use ffmpeg to convert to WAV (16kHz mono for optimal whisper performance)
		convertCmd := exec.Command("ffmpeg", "-i", tmpPath, "-ar", "16000", "-ac", "1", "-y", wavPath)
		var convertErr strings.Builder
		convertCmd.Stderr = &convertErr
		if err := convertCmd.Run(); err != nil {
			return "", fmt.Errorf("failed to convert audio to WAV: %w (stderr: %s)", err, convertErr.String())
		}
		audioPath = wavPath
	}

	// Execute whisper-cli command
	// whisper-cli -f <file> -m <model> -otxt -of <output-path>
	tmpDir, err := os.MkdirTemp("", "whisper-output-*")
	if err != nil {
		return "", fmt.Errorf("failed to create output dir: %w", err)
	}
	defer os.RemoveAll(tmpDir) // Clean up

	outputPath := filepath.Join(tmpDir, "transcript")

	cmd := exec.Command("whisper-cli",
		"-f", audioPath,
		"-m", w.model,
		"-otxt",
		"-of", outputPath,
		"-np", // no prints (quiet mode)
	)

	// Capture stderr for errors
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("whisper-cli command failed: %w (stderr: %s)", err, stderr.String())
	}

	// Read output file (whisper-cli creates <output-path>.txt)
	transcript, err := os.ReadFile(outputPath + ".txt")
	if err != nil {
		return "", fmt.Errorf("failed to read transcript: %w", err)
	}

	// Trim whitespace
	text := strings.TrimSpace(string(transcript))
	if text == "" {
		return "", fmt.Errorf("empty transcription (audio may be silent or invalid)")
	}

	return text, nil
}
