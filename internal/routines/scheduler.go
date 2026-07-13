// Package routines implements agentctl's in-process routine scheduler: an
// agent's own `agentctl serve` daemon periodically checking routines/*.toml
// for anything due and firing their command. There is no system crontab
// involved - a routine only ever lives inside its owning agent's process
// memory, and only fires while that daemon is running.
package routines

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/DeprecatedLuar/agentctl/internal/agent"
	"github.com/DeprecatedLuar/agentctl/internal/config"
	"github.com/DeprecatedLuar/agentctl/internal/logger"
	"github.com/DeprecatedLuar/agentctl/internal/resolution"
	"github.com/fsnotify/fsnotify"
)

const (
	routinesDirName  = "routines"
	tickInterval     = 60 * time.Second
	debounceInterval = 200 * time.Millisecond
)

type Scheduler struct {
	agentPath    string
	routinesPath string
	configEnv    map[string]string
	lg           *slog.Logger
	debug        bool
	startTime    time.Time

	mu             sync.Mutex
	routines       []config.RoutineConfig
	nextFire       map[string]time.Time // ModeDayInterval / ModeHourInterval
	lastFiredDate  map[string]string    // ModeWeekday / ModeDayOfMonth, dedupe key = "YYYY-MM-DD"
	lastFiredRRule map[string]time.Time // ModeRRule
}

// Start checks whether <agentPath>/routines/ exists and is non-empty; if so,
// compiles the schedule set and starts the ticker + fsnotify goroutines,
// both tied to ctx. If not, it's a no-op - zero background cost for the
// common case of an agent with no routines. Adding a routines/ folder to an
// already-running daemon requires a restart to pick up (checked once here at
// startup, not re-checked).
//
// configEnv is the agent's resolved [environment] map, passed through to
// shell.Execute exactly like tools use it.
func Start(ctx context.Context, agentPath string, configEnv map[string]string, lg *slog.Logger, debug bool) {
	routinesPath := filepath.Join(agentPath, routinesDirName)
	entries, err := os.ReadDir(routinesPath)
	if err != nil || len(entries) == 0 {
		return
	}

	s := &Scheduler{
		agentPath:      agentPath,
		routinesPath:   routinesPath,
		configEnv:      configEnv,
		lg:             lg,
		debug:          debug,
		startTime:      time.Now(),
		nextFire:       make(map[string]time.Time),
		lastFiredDate:  make(map[string]string),
		lastFiredRRule: make(map[string]time.Time),
	}

	s.reload()

	go s.tickLoop(ctx)
	go s.watchLoop(ctx)
}

// reload fully recompiles the schedule set from disk. Runtime reload
// failures (a routine broken after the daemon already started) are logged
// and non-fatal - LoadRoutines already omits files with errors from the
// returned slice, matching the daemon's existing hot-reload philosophy of
// "malformed config logs a warning, daemon stays running."
func (s *Scheduler) reload() {
	routines, issues := config.LoadRoutines(s.agentPath)
	for _, issue := range issues {
		if issue.Type == config.IssueError {
			s.lg.Error(issue.Message)
		} else {
			s.lg.Warn(issue.Message)
		}
	}

	s.mu.Lock()
	s.routines = routines
	s.mu.Unlock()
}

func (s *Scheduler) tickLoop(ctx context.Context) {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.checkDue(time.Now())
		case <-ctx.Done():
			return
		}
	}
}

func (s *Scheduler) checkDue(now time.Time) {
	s.mu.Lock()
	routines := make([]config.RoutineConfig, len(s.routines))
	copy(routines, s.routines)
	s.mu.Unlock()

	for i := range routines {
		r := routines[i]
		if !r.Enabled {
			continue
		}
		if s.isDue(&r, now) {
			// Overlap is allowed by design - fire regardless of whether a
			// previous invocation of this routine is still running.
			go s.fire(r)
		}
	}
}

// isDue reports whether r should fire at now, advancing whatever per-routine
// "already fired" state applies to its mode as a side effect when it does.
func (s *Scheduler) isDue(r *config.RoutineConfig, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	sched := r.Schedule
	switch sched.Mode {
	case config.ModeHourInterval, config.ModeDayInterval:
		next, ok := s.nextFire[r.Name]
		if !ok {
			// First observation of this routine: don't fire immediately on
			// daemon start, wait a full interval from startTime.
			next = s.startTime.Add(sched.Interval)
			s.nextFire[r.Name] = next
		}
		if now.Before(next) {
			return false
		}
		for !next.After(now) {
			next = next.Add(sched.Interval)
		}
		s.nextFire[r.Name] = next
		return true

	case config.ModeWeekday, config.ModeDayOfMonth:
		matches := false
		if sched.Mode == config.ModeWeekday {
			for _, w := range sched.Weekdays {
				if now.Weekday() == w {
					matches = true
					break
				}
			}
		} else {
			for _, d := range sched.DaysOfMonth {
				if now.Day() == d {
					matches = true
					break
				}
			}
		}
		if !matches {
			return false
		}

		instant := time.Date(now.Year(), now.Month(), now.Day(), sched.Time.Hour(), sched.Time.Minute(), 0, 0, now.Location())
		if instant.Before(s.startTime) {
			return false // never fire an occurrence scheduled before the daemon started
		}
		elapsed := now.Sub(instant)
		if elapsed < 0 || elapsed >= tickInterval {
			return false // outside this tick's window
		}

		key := instant.Format("2006-01-02")
		if s.lastFiredDate[r.Name] == key {
			return false // already fired this occurrence
		}
		s.lastFiredDate[r.Name] = key
		return true

	case config.ModeRRule:
		if sched.RRuleSet == nil {
			return false
		}
		after, ok := s.lastFiredRRule[r.Name]
		if !ok {
			after = s.startTime
		}
		occurrence := sched.RRuleSet.After(after, false)
		if occurrence.IsZero() || occurrence.After(now) {
			return false
		}
		s.lastFiredRRule[r.Name] = occurrence
		return true
	}

	return false
}

// fire re-reads the routine fresh off disk immediately before executing, as
// a final freshness gate against a delete/disable/edit that landed after the
// last fsnotify-driven reload but before this fire moment.
func (s *Scheduler) fire(r config.RoutineConfig) {
	fresh, issues := config.LoadRoutine(r.Path, s.routinesPath)
	for _, issue := range issues {
		if issue.Type == config.IssueError {
			s.lg.Warn(r.Name, "kind", logger.KindTool, "error", issue.Message)
			return
		}
	}
	if !fresh.Enabled {
		return
	}

	runtimeCtx := resolution.Context{
		AgentPath: s.agentPath,
		AgentName: filepath.Base(s.agentPath),
		Timestamp: time.Now(),
		ConfigEnv: s.configEnv,
	}

	agent.ExecuteRoutine(&fresh, s.agentPath, runtimeCtx, s.lg, s.debug)
}

// watchLoop watches the routines/ directory itself, not individual files -
// directory-level watching survives editors that save via temp-file +
// rename(), which swaps the inode a file-level watch would be attached to.
// Events are filtered to *.toml and debounced so one edit collapses into one
// reload.
func (s *Scheduler) watchLoop(ctx context.Context) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		s.lg.Warn("routines: failed to start file watcher", "error", err)
		return
	}
	defer watcher.Close()

	if err := watcher.Add(s.routinesPath); err != nil {
		s.lg.Warn("routines: failed to watch directory", "path", s.routinesPath, "error", err)
		return
	}

	var debounce *time.Timer
	reload := make(chan struct{}, 1)

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if !strings.HasSuffix(event.Name, ".toml") {
				continue
			}
			if debounce != nil {
				debounce.Stop()
			}
			debounce = time.AfterFunc(debounceInterval, func() {
				select {
				case reload <- struct{}{}:
				default:
				}
			})
		case <-reload:
			s.reload()
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			s.lg.Warn("routines: file watcher error", "error", err)
		case <-ctx.Done():
			return
		}
	}
}
