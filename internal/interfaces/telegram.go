package interfaces

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/DeprecatedLuar/agentctl/internal/agent"
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
}

// NewTelegram creates a new Telegram interface
func NewTelegram(agentFolder string) (*TelegramInterface, error) {
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
	}, nil
}

// Start begins the Telegram bot polling loop
func (t *TelegramInterface) Start(ctx context.Context, runner *Runner) error {
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

			go t.handleMessage(ctx, bot, update.Message, runner)
		}
	}
}

func (t *TelegramInterface) handleMessage(ctx context.Context, bot *tgbotapi.BotAPI, message *tgbotapi.Message, runner *Runner) {
	chatID := message.Chat.ID
	userID := message.From.ID
	text := message.Text

	sessionKey := fmt.Sprintf("telegram_%d", userID)

	// Handle /start command
	if text == "/start" {
		welcomeMsg := "Agent ready. Send a message to begin."
		msg := tgbotapi.NewMessage(chatID, welcomeMsg)
		bot.Send(msg)
		if runner.Logger != nil {
			runner.Logger.Info("start command", "user", sessionKey)
		}
		return
	}

	// Log message received
	if runner.Logger != nil {
		msg := fmt.Sprintf("message received %s:%s", sessionKey, interfaceNameTelegram)
		if runner.Verbose {
			runner.Logger.Info(msg, "content", text)
		} else {
			runner.Logger.Info(msg)
		}
	}

	// Start typing indicator
	typingCtx, cancelTyping := context.WithCancel(ctx)
	go t.sendTypingLoop(typingCtx, bot, chatID)

	// Run agent
	response, err := runner.Run(agent.Input{
		SessionKey: sessionKey,
		Content:    text,
	})

	// Stop typing indicator
	cancelTyping()

	// Send response or error
	if err != nil {
		if runner.Logger != nil {
			runner.Logger.Error(fmt.Sprintf("agent error %s:%s", sessionKey, interfaceNameTelegram), "error", err)
		}
		errorMsg := fmt.Sprintf("Error: %v", err)
		msg := tgbotapi.NewMessage(chatID, errorMsg)
		bot.Send(msg)
		return
	}

	// Log response sent
	if runner.Logger != nil {
		msg := fmt.Sprintf("response sent %s:%s", sessionKey, interfaceNameTelegram)
		if runner.Verbose {
			runner.Logger.Info(msg, "content", response)
		} else {
			runner.Logger.Info(msg)
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
