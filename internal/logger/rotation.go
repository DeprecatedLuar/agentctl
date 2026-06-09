package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const maxFileSize = 17 * 1024 * 1024 // 17MB

// RotatingWriter wraps a file with size-based rotation
type RotatingWriter struct {
	path    string
	file    *os.File
	written int64
}

// NewRotatingWriter creates a rotating file writer
func NewRotatingWriter(path string) (*RotatingWriter, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}

	// Open or create log file
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	// Get current size
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("stat log file: %w", err)
	}

	return &RotatingWriter{
		path:    path,
		file:    file,
		written: info.Size(),
	}, nil
}

// Write implements io.Writer with rotation check
func (w *RotatingWriter) Write(p []byte) (n int, err error) {
	// Check if rotation needed before write
	if w.written+int64(len(p)) > maxFileSize {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}

	n, err = w.file.Write(p)
	w.written += int64(n)
	return n, err
}

// rotate performs log rotation: agent.log → agent.log.1 → agent.log.2
func (w *RotatingWriter) rotate() error {
	// Close current file
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("close current log: %w", err)
	}

	// Rotate existing files
	log2 := w.path + ".2"
	log1 := w.path + ".1"

	// Remove oldest if exists
	os.Remove(log2)

	// Rename log.1 → log.2
	if _, err := os.Stat(log1); err == nil {
		if err := os.Rename(log1, log2); err != nil {
			return fmt.Errorf("rotate log.1 to log.2: %w", err)
		}
	}

	// Rename log → log.1
	if err := os.Rename(w.path, log1); err != nil {
		return fmt.Errorf("rotate log to log.1: %w", err)
	}

	// Create new log file
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("create new log file: %w", err)
	}

	w.file = file
	w.written = 0
	return nil
}

// Close closes the underlying file
func (w *RotatingWriter) Close() error {
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

// Ensure RotatingWriter implements io.WriteCloser
var _ io.WriteCloser = (*RotatingWriter)(nil)
