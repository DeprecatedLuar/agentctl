package interfaces

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/DeprecatedLuar/agentctl/internal/providers/audio"
	"github.com/DeprecatedLuar/agentctl/internal/session"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

const (
	// Telegram configuration
	telegramEnvFile      = ".env"
	telegramBotTokenKey  = "TELEGRAM_BOT_TOKEN"
	telegramPollTimeout  = 60
	typingInterval       = 4 * time.Second
	interfaceNameTelegram = "telegram"
)

// TelegramInterface implements the Telegram bot interface
type TelegramInterface struct {
	agentFolder string
	token       string
	transcriber audio.Transcriber // Optional, for voice message support
	handler     MessageHandler
	store       session.SessionStore
	logger      *slog.Logger
	verbose     bool
}

// NewTelegram creates a new Telegram interface
func NewTelegram(agentFolder string, transcriber audio.Transcriber, handler MessageHandler, store session.SessionStore, logger *slog.Logger, verbose bool) (*TelegramInterface, error) {
	// Load .env from agent folder
	envPath := filepath.Join(agentFolder, telegramEnvFile)
	_ = godotenv.Load(envPath)

	token := os.Getenv(telegramBotTokenKey)
	if token == "" {
		return nil, fmt.Errorf("%s not found in %s or environment", telegramBotTokenKey, telegramEnvFile)
	}

	return &TelegramInterface{
		agentFolder: agentFolder,
		token:       token,
		transcriber: transcriber,
		handler:     handler,
		store:       store,
		logger:      logger,
		verbose:     verbose,
	}, nil
}

// Start begins the Telegram bot polling loop
func (t *TelegramInterface) Start(ctx context.Context) error {
	// Create bot API instance
	bot, err := tgbotapi.NewBotAPI(t.token)
	if err != nil {
		return fmt.Errorf("failed to create telegram bot: %w", err)
	}

	fmt.Printf("Telegram interface authorized as @%s\n", bot.Self.UserName)

	// Configure updates
	u := tgbotapi.NewUpdate(0)
	u.Timeout = telegramPollTimeout

	updates := bot.GetUpdatesChan(u)

	// Poll loop
	for {
		select {
		case <-ctx.Done():
			bot.StopReceivingUpdates()
			return nil
		case update := <-updates:
			if update.Message == nil {
				continue
			}

			go t.handleMessage(ctx, bot, update.Message)
		}
	}
}

func (t *TelegramInterface) handleMessage(ctx context.Context, bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	chatID := message.Chat.ID
	userID := message.From.ID
	text := message.Text

	// Resolve session
	contactID := strconv.FormatInt(userID, 10)
	displayName := message.From.UserName
	resolved, err := session.Resolve(t.store, t.agentFolder, interfaceNameTelegram, contactID, displayName)
	if err != nil {
		if t.logger != nil {
			t.logger.Error("session resolution failed", "contact", contactID, "error", err)
		}
		errorMsg := fmt.Sprintf("Session error: %v", err)
		msg := tgbotapi.NewMessage(chatID, errorMsg)
		bot.Send(msg)
		return
	}

	// Handle /start command
	if text == "/start" {
		welcomeMsg := "Agent ready. Send a message to begin."
		msg := tgbotapi.NewMessage(chatID, welcomeMsg)
		bot.Send(msg)
		if t.logger != nil {
			t.logger.Info("start command", "user", resolved.UserID)
		}
		return
	}

	// Handle voice messages
	if message.Voice != nil && t.transcriber != nil {
		var transcribeErr error
		text, transcribeErr = t.transcribeVoiceMessage(bot, message.Voice)
		if transcribeErr != nil {
			if t.logger != nil {
				t.logger.Error(fmt.Sprintf("transcription error %s:%s", resolved.UserID, interfaceNameTelegram), "error", transcribeErr)
			}
			errorMsg := fmt.Sprintf("Failed to transcribe voice message: %v", transcribeErr)
			msg := tgbotapi.NewMessage(chatID, errorMsg)
			bot.Send(msg)
			return
		}
		if t.logger != nil {
			t.logger.Info("voice message transcribed", "user", resolved.UserID, "text", text)
		}
	}

	// Log message received
	if t.logger != nil {
		msg := fmt.Sprintf("message received %s:%s", resolved.UserID, interfaceNameTelegram)
		if t.verbose {
			t.logger.Info(msg, "content", text)
		} else {
			t.logger.Info(msg)
		}
	}

	// Start typing indicator
	typingCtx, cancelTyping := context.WithCancel(ctx)
	go t.sendTypingLoop(typingCtx, bot, chatID)

	// Handle message via abstraction
	response, runErr := t.handler.HandleMessage(MessageOptions{
		UserID:    resolved.UserID,
		SessionID: resolved.SessionID,
		Interface: interfaceNameTelegram,
		Content:   text,
	})

	// Stop typing indicator
	cancelTyping()

	// Send response or error
	if runErr != nil {
		if t.logger != nil {
			t.logger.Error(fmt.Sprintf("agent error %s:%s", resolved.UserID, interfaceNameTelegram), "error", runErr)
		}
		errorMsg := fmt.Sprintf("Error: %v", runErr)
		msg := tgbotapi.NewMessage(chatID, errorMsg)
		bot.Send(msg)
		return
	}

	// Log response sent
	if t.logger != nil {
		msg := fmt.Sprintf("response sent %s:%s", resolved.UserID, interfaceNameTelegram)
		if t.verbose {
			t.logger.Info(msg, "content", response)
		} else {
			t.logger.Info(msg)
		}
	}

	msg := tgbotapi.NewMessage(chatID, response)
	bot.Send(msg)
}

func (t *TelegramInterface) sendTypingLoop(ctx context.Context, bot *tgbotapi.BotAPI, chatID int64) {
	// Send typing immediately
	action := tgbotapi.NewChatAction(chatID, "typing")
	bot.Send(action)

	ticker := time.NewTicker(typingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			action := tgbotapi.NewChatAction(chatID, "typing")
			bot.Send(action)
		case <-ctx.Done():
			return
		}
	}
}

func (t *TelegramInterface) transcribeVoiceMessage(bot *tgbotapi.BotAPI, voice *tgbotapi.Voice) (string, error) {
	// Get file download URL
	fileConfig := tgbotapi.FileConfig{FileID: voice.FileID}
	file, err := bot.GetFile(fileConfig)
	if err != nil {
		return "", fmt.Errorf("failed to get file info: %w", err)
	}

	fileURL := file.Link(bot.Token)

	// Download the file
	resp, err := http.Get(fileURL)
	if err != nil {
		return "", fmt.Errorf("failed to download voice file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download voice file: HTTP %d", resp.StatusCode)
	}

	// Read audio data
	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read audio data: %w", err)
	}

	// Transcribe
	text, err := t.transcriber.Transcribe(audioData, "voice.ogg")
	if err != nil {
		return "", fmt.Errorf("transcription failed: %w", err)
	}

	return text, nil
}
