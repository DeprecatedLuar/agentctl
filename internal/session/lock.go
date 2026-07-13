package session

import (
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
)

// LockSession acquires an exclusive, cross-process lock for (userID,
// sessionID), backed by a per-session sidecar file
// (.data/sessions/{userID}/.{sessionID}.lock). It is the outer layer above
// the in-process queue/mutex: same-process callers are already serialized
// before this is ever reached, so contention here only happens between
// separate processes (e.g. a `serve` daemon and a one-shot `chat` touching
// the same session). Callers must hold it for the entire operation (turn,
// inject, or title SetMeta) and call the returned unlock when done.
func LockSession(agentFolder, userID, sessionID string, logger *slog.Logger) (unlock func(), err error) {
	dir := UserFolder(agentFolder, userID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	lockPath := filepath.Join(dir, "."+sessionID+".lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if logger != nil {
			logger.Info("session busy, waiting", "session", sessionID)
		}
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
			f.Close()
			return nil, err
		}
	}

	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
