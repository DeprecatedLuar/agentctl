package telegram

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/DeprecatedLuar/agentctl/internal"
	"github.com/DeprecatedLuar/agentctl/internal/providers/audio"
	"github.com/DeprecatedLuar/agentctl/internal/session"
	"github.com/DeprecatedLuar/agentctl/internal/syscommands"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
	"time"
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
	handler     internal.MessageHandler
	store       session.SessionStore // For command handling
	logger      *slog.Logger
	verbose     bool
	bot         *tgbotapi.BotAPI // Initialized in Start, used by Sender interface
}

// NewTelegram creates a new Telegram interface
func NewTelegram(agentFolder string, transcriber audio.Transcriber, handler internal.MessageHandler, store session.SessionStore, logger *slog.Logger, verbose bool) (*TelegramInterface, error) {
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

	// Store bot for Sender interface
	t.bot = bot

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

	// Extract contact information
	contactID := strconv.FormatInt(userID, 10)

	// Build display name from FirstName + LastName
	displayName := message.From.FirstName
	if message.From.LastName != "" {
		displayName += " " + message.From.LastName
	}

	// Extract @username handle (optional)
	username := message.From.UserName

	// Handle /start command
	if text == "/start" {
		welcomeMsg := "Agent ready. Send a message to begin."
		msg := tgbotapi.NewMessage(chatID, welcomeMsg)
		bot.Send(msg)
		if t.logger != nil {
			t.logger.Info("start command", "contact", contactID)
		}
		return
	}

	// Handle voice messages
	if message.Voice != nil && t.transcriber != nil {
		var transcribeErr error
		text, transcribeErr = t.transcribeVoiceMessage(bot, message.Voice)
		if transcribeErr != nil {
			if t.logger != nil {
				t.logger.Error("transcription error", "contact", contactID, "error", transcribeErr)
			}
			errorMsg := fmt.Sprintf("Failed to transcribe voice message: %v", transcribeErr)
			msg := tgbotapi.NewMessage(chatID, errorMsg)
			bot.Send(msg)
			return
		}
		if t.logger != nil {
			t.logger.Info("voice message transcribed", "contact", contactID, "text", text)
		}
	}

	// Check if message is a command (but not /start, already handled above)
	cmd, cmdErr := syscommands.Parse(text)
	if cmdErr == nil {
		// Handle command in Telegram layer (no typing indicator for commands)
		response, err := t.handleCommand(cmd, contactID)
		if err != nil {
			errorMsg := fmt.Sprintf("Error: %v", err)
			msg := tgbotapi.NewMessage(chatID, errorMsg)
			bot.Send(msg)
		} else {
			msg := tgbotapi.NewMessage(chatID, response)
			bot.Send(msg)
		}
		return
	}

	// Log message received
	if t.logger != nil {
		msg := fmt.Sprintf("message received %s:%s", contactID, interfaceNameTelegram)
		if t.verbose {
			t.logger.Info(msg, "content", text)
		} else {
			t.logger.Info(msg)
		}
	}

	// Start typing indicator
	typingCtx, cancelTyping := context.WithCancel(ctx)
	go t.sendTypingLoop(typingCtx, bot, chatID)

	// Handle message via service layer (pure I/O adapter)
	response, runErr := t.handler.HandleMessage(interfaceNameTelegram, contactID, displayName, username, text)

	// Stop typing indicator
	cancelTyping()

	// Send response or error
	if runErr != nil {
		if t.logger != nil {
			t.logger.Error("agent error", "contact", contactID, "interface", interfaceNameTelegram, "error", runErr)
		}
		errorMsg := fmt.Sprintf("Error: %v", runErr)
		msg := tgbotapi.NewMessage(chatID, errorMsg)
		bot.Send(msg)
		return
	}

	// Log response sent
	if t.logger != nil {
		msg := fmt.Sprintf("response sent %s:%s", contactID, interfaceNameTelegram)
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

// InterfaceName returns the interface identifier for Sender interface
func (t *TelegramInterface) InterfaceName() string {
	return interfaceNameTelegram
}

// Send delivers a message to the specified Telegram chat ID (Sender interface)
func (t *TelegramInterface) Send(platformID, content string) error {
	if t.bot == nil {
		return fmt.Errorf("telegram bot not initialized")
	}

	chatID, err := strconv.ParseInt(platformID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid telegram chat ID %q: %w", platformID, err)
	}

	msg := tgbotapi.NewMessage(chatID, content)
	_, err = t.bot.Send(msg)
	return err
}
