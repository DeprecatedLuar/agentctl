package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/DeprecatedLuar/agentctl/internal/registry"
)

// Timing for graceful shutdown before escalating to SIGKILL.
const (
	stopPollInterval = 100 * time.Millisecond
	stopGraceTimeout = 5 * time.Second
)

// HandleStop signals a running `agentctl run` daemon to shut down.
// With no flags: finds the agent in the current directory tree.
// With -a/--agent: resolves the provided name/path.
func HandleStop(args []string) error {
	var agentPath string
	var err error

	if len(args) > 0 && (args[0] == flagAgent || args[0] == flagAgentS) {
		if len(args) < 2 {
			return fmt.Errorf("%s requires a path or name", args[0])
		}
		agentPath, err = registry.ResolveAgentPath(args[1])
		if err != nil {
			return err
		}
	} else {
		agentPath, err = registry.FindAgentInPath()
		if err != nil {
			return err
		}
	}

	agentName := filepath.Base(agentPath)
	lockPath := filepath.Join(agentPath, dataDir, lockFile)

	pid, err := readLockPID(lockPath)
	if err != nil {
		return fmt.Errorf("agent %q is not running", agentName)
	}

	// Confirm the process is actually alive before signaling.
	if err := syscall.Kill(pid, 0); err != nil {
		return fmt.Errorf("agent %q is not running (stale lock)", agentName)
	}

	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to signal agent %q (pid %d): %w", agentName, pid, err)
	}

	deadline := time.Now().Add(stopGraceTimeout)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) != nil {
			fmt.Printf("agent %q stopped\n", agentName)
			return nil
		}
		time.Sleep(stopPollInterval)
	}

	// Didn't exit in time - force kill.
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		return fmt.Errorf("agent %q did not stop gracefully and kill failed: %w", agentName, err)
	}
	fmt.Printf("agent %q did not stop gracefully, killed (pid %d)\n", agentName, pid)
	return nil
}

// readLockPID reads and parses the PID written by `run.go` into the lock
// file. Returns an error if the file is missing, empty, or malformed.
func readLockPID(lockPath string) (int, error) {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return 0, err
	}
	s := strings.TrimSpace(string(data))
	if s == "" {
		return 0, fmt.Errorf("lock file is empty")
	}
	pid, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid pid in lock file: %w", err)
	}
	return pid, nil
}
