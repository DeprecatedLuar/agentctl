package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

const (
	// LogFileName is the on-disk log file name, shared with commands/monitor.go.
	LogFileName = "agent.log"
)

// Setup creates a logger with stdout output (pretty, TTY-aware), optionally
// adding file logging. Used by `serve` - interactive daemon output stays
// human-formatted on stdout; the file, if enabled, is JSON so `monitor` can
// parse it back into the same pretty format live.
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
		fileHandler, err := newFileHandler(logDir, opts)
		if err != nil {
			return nil, err
		}

		// Use multi-handler to write to both
		handler = &multiHandler{
			handlers: []slog.Handler{stdoutHandler, fileHandler},
		}
	}

	logger := slog.New(handler)
	return logger, nil
}

// SetupOneShot builds a logger for one-shot commands (chat/deliver). Unlike
// Setup, nothing goes to stdout - the scriptability contract reserves
// stdout for the response only. Normal logs land only in the file (JSON,
// same as Setup); warn/error always surface on stderr so failures are
// visible even with file logging off, and --debug bumps the stderr level to
// show everything.
func SetupOneShot(logDir string, debug bool, enableFileLogging bool) (*slog.Logger, error) {
	stderrLevel := slog.LevelWarn
	if debug {
		stderrLevel = slog.LevelDebug
	}
	stderrHandler := NewPrettyHandler(os.Stderr, &slog.HandlerOptions{Level: stderrLevel})

	var handler slog.Handler = stderrHandler

	if enableFileLogging {
		fileLevel := slog.LevelInfo
		if debug {
			fileLevel = slog.LevelDebug
		}
		fileHandler, err := newFileHandler(logDir, &slog.HandlerOptions{Level: fileLevel})
		if err != nil {
			return nil, err
		}

		handler = &multiHandler{
			handlers: []slog.Handler{stderrHandler, fileHandler},
		}
	}

	return slog.New(handler), nil
}

// newFileHandler builds the rotating, JSON-formatted file handler shared by
// Setup and SetupOneShot.
func newFileHandler(logDir string, opts *slog.HandlerOptions) (slog.Handler, error) {
	logPath := filepath.Join(logDir, LogFileName)
	rotatingWriter, err := NewRotatingWriter(logPath)
	if err != nil {
		return nil, fmt.Errorf("setup rotating writer: %w", err)
	}
	return slog.NewJSONHandler(rotatingWriter, opts), nil
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
		// Each sub-handler's own level must be checked here: multiHandler's
		// own Enabled() only reports whether ANY sub-handler wants a level,
		// so sub-handlers can (and now do, for SetupOneShot) run at
		// different levels from each other.
		if !handler.Enabled(ctx, r.Level) {
			continue
		}
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
