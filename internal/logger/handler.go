package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
)

// Colors for TTY output
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
)

// PrettyHandler formats logs for human readability when output is a TTY
type PrettyHandler struct {
	mu      sync.Mutex
	writer  io.Writer
	level   slog.Level
	isTTY   bool
	groups  []string
	attrs   []slog.Attr
}

// NewPrettyHandler creates a handler that formats nicely for TTY
func NewPrettyHandler(w io.Writer, opts *slog.HandlerOptions) *PrettyHandler {
	level := slog.LevelInfo
	if opts != nil && opts.Level != nil {
		level = opts.Level.Level()
	}

	// Check if output is a TTY
	isTTY := false
	if f, ok := w.(*os.File); ok {
		stat, err := f.Stat()
		if err == nil {
			isTTY = (stat.Mode() & os.ModeCharDevice) != 0
		}
	}

	return &PrettyHandler{
		writer: w,
		level:  level,
		isTTY:  isTTY,
	}
}

func (h *PrettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *PrettyHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	var output string

	if h.isTTY {
		// Pretty format for TTY: LEVEL HH:MM:SS message key=val
		levelStr := h.formatLevel(r.Level)
		timeStr := r.Time.Format("15:04:05")

		output = fmt.Sprintf("%s %s%s%s %s",
			levelStr,
			colorGray, timeStr, colorReset,
			r.Message,
		)

		// Add attributes
		r.Attrs(func(a slog.Attr) bool {
			output += fmt.Sprintf(" %s%s%s=%v",
				colorCyan, a.Key, colorReset, a.Value,
			)
			return true
		})

		output += "\n"
	} else {
		// Structured format for pipes/non-TTY
		output = fmt.Sprintf("time=%s level=%s msg=%q",
			r.Time.Format("2006-01-02T15:04:05.000Z07:00"),
			r.Level.String(),
			r.Message,
		)

		r.Attrs(func(a slog.Attr) bool {
			output += fmt.Sprintf(" %s=%v", a.Key, a.Value)
			return true
		})

		output += "\n"
	}

	_, err := h.writer.Write([]byte(output))
	return err
}

func (h *PrettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)
	return &PrettyHandler{
		writer: h.writer,
		level:  h.level,
		isTTY:  h.isTTY,
		groups: h.groups,
		attrs:  newAttrs,
	}
}

func (h *PrettyHandler) WithGroup(name string) slog.Handler {
	newGroups := make([]string, len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups[len(h.groups)] = name
	return &PrettyHandler{
		writer: h.writer,
		level:  h.level,
		isTTY:  h.isTTY,
		groups: newGroups,
		attrs:  h.attrs,
	}
}

func (h *PrettyHandler) formatLevel(level slog.Level) string {
	if !h.isTTY {
		return level.String()
	}

	switch level {
	case slog.LevelDebug:
		return colorGray + "[DEBUG]" + colorReset
	case slog.LevelInfo:
		return colorBlue + "[INFO]" + colorReset
	case slog.LevelWarn:
		return colorYellow + "[WARN]" + colorReset
	case slog.LevelError:
		return colorRed + "[ERROR]" + colorReset
	default:
		return "[" + level.String() + "]"
	}
}
