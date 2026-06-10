package main

import (
	"fmt"
	"os"

	"github.com/DeprecatedLuar/agentctl/internal/config"
	"github.com/DeprecatedLuar/agentctl/internal/providers/audio"
)

func main() {
	audioPath := "/home/luar/Media/rec/audio/26-06-10_08-32_mic.wav"

	// Read audio file
	audioData, err := os.ReadFile(audioPath)
	if err != nil {
		fmt.Printf("Failed to read audio file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Loaded audio file: %s (%d bytes)\n", audioPath, len(audioData))

	// Create whisper CLI provider
	cfg := &config.AudioConfig{
		Provider: "whisper-cli",
		Model:    "/tmp/whisper-models/ggml-base.bin",
	}

	transcriber, err := audio.New(cfg, ".")
	if err != nil {
		fmt.Printf("Failed to create transcriber: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Transcribing...")

	// Transcribe
	text, err := transcriber.Transcribe(audioData, "audio.wav")
	if err != nil {
		fmt.Printf("Transcription failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n=== Transcription ===")
	fmt.Println(text)
	fmt.Println("====================")
}
