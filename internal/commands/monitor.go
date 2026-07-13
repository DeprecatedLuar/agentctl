package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/DeprecatedLuar/agentctl/internal/logger"
	"github.com/DeprecatedLuar/agentctl/internal/registry"
	"github.com/fsnotify/fsnotify"
)

// HandleMonitor follows an agent's log file live, rendering each JSON line
// through the same PrettyHandler formatting `serve` uses for stdout. It's
// the read side of the JSON file format: any process writing to agent.log
// (serve, or a one-shot chat/deliver) shows up here, so this works with no
// gateways running at all.
func HandleMonitor(args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	absPath, err := registry.ResolveAgentPath(path)
	if err != nil {
		return err
	}

	logDir := filepath.Join(absPath, dataDir, logsDir)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("create log directory: %w", err)
	}
	logPath := filepath.Join(logDir, logger.LogFileName)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to start file watcher: %w", err)
	}
	defer watcher.Close()

	if err := watcher.Add(logDir); err != nil {
		return fmt.Errorf("failed to watch log directory: %w", err)
	}

	t := &logTailer{path: logPath, handler: logger.NewPrettyHandler(os.Stdout, nil)}
	t.drain() // catch up to current EOF - don't dump prior history

	fmt.Fprintf(os.Stderr, "monitoring %s (ctrl-c to stop)\n", logPath)

	for event := range watcher.Events {
		if filepath.Base(event.Name) != logger.LogFileName {
			continue
		}
		// Create/Rename fire when rotation swaps agent.log for a fresh file
		// (see logger.RotatingWriter.rotate) - reopen from scratch.
		if event.Op&(fsnotify.Create|fsnotify.Rename|fsnotify.Remove) != 0 {
			t.reopen()
		}
		t.drain()
	}
	return nil
}

// logTailer incrementally reads newly appended lines from a log file,
// tolerating the file not existing yet and being rotated out from under it.
type logTailer struct {
	path       string
	handler    slog.Handler
	file       *os.File
	buf        []byte // unconsumed partial line
	everOpened bool
}

// ensureOpen opens the file if not already open. On the very first open it
// seeks to EOF (skip pre-existing history); a reopen after rotation starts
// at the beginning of the new file.
func (t *logTailer) ensureOpen() bool {
	if t.file != nil {
		return true
	}
	f, err := os.Open(t.path)
	if err != nil {
		return false
	}
	if !t.everOpened {
		f.Seek(0, io.SeekEnd)
	}
	t.everOpened = true
	t.file = f
	return true
}

func (t *logTailer) reopen() {
	if t.file != nil {
		t.file.Close()
		t.file = nil
	}
	t.buf = nil
}

// drain reads and renders every complete line currently available, leaving
// any trailing partial line buffered for the next call.
func (t *logTailer) drain() {
	if !t.ensureOpen() {
		return
	}

	chunk := make([]byte, 64*1024)
	for {
		n, err := t.file.Read(chunk)
		if n > 0 {
			t.buf = append(t.buf, chunk[:n]...)
			for {
				i := bytes.IndexByte(t.buf, '\n')
				if i < 0 {
					break
				}
				line := t.buf[:i]
				t.buf = t.buf[i+1:]
				if len(bytes.TrimSpace(line)) > 0 {
					renderLogLine(t.handler, line)
				}
			}
		}
		if err != nil {
			break // EOF (or a transient read error) - wait for the next event
		}
	}
}

// renderLogLine parses one JSON log line and replays it through handler as
// a slog.Record, so it renders identically to how `serve` would have shown
// it live on stdout. Malformed lines (e.g. a torn write caught mid-rotation)
// are skipped rather than surfaced as errors.
func renderLogLine(handler slog.Handler, line []byte) {
	keys, values, err := decodeOrderedJSON(line)
	if err != nil {
		return
	}

	var ts time.Time
	var level slog.Level
	var msg string

	if raw, ok := values["time"]; ok {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			ts, _ = time.Parse(time.RFC3339Nano, s)
		}
	}
	if raw, ok := values["level"]; ok {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			_ = level.UnmarshalText([]byte(s))
		}
	}
	if raw, ok := values["msg"]; ok {
		_ = json.Unmarshal(raw, &msg)
	}

	r := slog.NewRecord(ts, level, msg, 0)
	for _, k := range keys {
		if k == "time" || k == "level" || k == "msg" {
			continue
		}
		var v any
		_ = json.Unmarshal(values[k], &v)
		r.AddAttrs(slog.Any(k, v))
	}

	_ = handler.Handle(context.Background(), r)
}

// decodeOrderedJSON decodes a flat JSON object, preserving key order (which
// plain map decoding does not) so attrs render in the order they were
// logged - e.g. tool-call args reading in the same order as the source
// logger.Info call.
func decodeOrderedJSON(line []byte) (keys []string, values map[string]json.RawMessage, err error) {
	dec := json.NewDecoder(bytes.NewReader(line))
	tok, err := dec.Token()
	if err != nil {
		return nil, nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, nil, fmt.Errorf("expected a JSON object")
	}

	values = make(map[string]json.RawMessage)
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, nil, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, nil, fmt.Errorf("expected string key")
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, nil, err
		}
		keys = append(keys, key)
		values[key] = raw
	}
	return keys, values, nil
}
