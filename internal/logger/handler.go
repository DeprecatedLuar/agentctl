package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
)

const attrKind = "kind"

// PrettyHandler formats logs for human readability when output is a TTY
type PrettyHandler struct {
	mu     sync.Mutex
	writer io.Writer
	level  slog.Level
	isTTY  bool
	groups []string
	attrs  []slog.Attr
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

	// Pull the "kind" attr out (if any) so it drives the tag instead of
	// printing as a regular key=val pair.
	var kind string
	attrs := make([]slog.Attr, 0, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == attrKind {
			kind = a.Value.String()
			return true
		}
		attrs = append(attrs, a)
		return true
	})

	output := h.format(r, kind, attrs)

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
