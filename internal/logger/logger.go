package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

const (
	// Log file name
	logFileName = "agent.log"
)

// Setup creates a logger with stdout output, optionally adding file logging
func Setup(logDir string, verbose bool, debug bool, enableFileLogging bool) (*slog.Logger, error) {
	// Determine log level
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	// Create pretty handler for stdout (TTY-aware)
	stdoutHandler := NewPrettyHandler(os.Stdout, opts)

	var handler slog.Handler = stdoutHandler

	// Add file logging if enabled
	if enableFileLogging {
		logPath := filepath.Join(logDir, logFileName)
		rotatingWriter, err := NewRotatingWriter(logPath)
		if err != nil {
			return nil, fmt.Errorf("setup rotating writer: %w", err)
		}

		// File handler uses structured format
		fileHandler := slog.NewTextHandler(rotatingWriter, opts)

		// Use multi-handler to write to both
		handler = &multiHandler{
			handlers: []slog.Handler{stdoutHandler, fileHandler},
		}
	}

	logger := slog.New(handler)
	return logger, nil
}

// multiHandler writes to multiple handlers
type multiHandler struct {
	handlers []slog.Handler
}

func (h *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, handler := range h.handlers {
		if err := handler.Handle(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

func (h *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithAttrs(attrs)
	}
	return &multiHandler{handlers: newHandlers}
}

func (h *multiHandler) WithGroup(name string) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithGroup(name)
	}
	return &multiHandler{handlers: newHandlers}
}
