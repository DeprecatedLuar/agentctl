package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/DeprecatedLuar/agentctl/internal"
	"github.com/DeprecatedLuar/agentctl/internal/config"
	"github.com/DeprecatedLuar/agentctl/internal/logger"
	"github.com/DeprecatedLuar/agentctl/internal/providers/audio"
	"github.com/DeprecatedLuar/agentctl/internal/registry"
	"github.com/DeprecatedLuar/agentctl/internal/resolution"
	"github.com/DeprecatedLuar/agentctl/internal/routines"
	"github.com/DeprecatedLuar/agentctl/internal/session"
)

const (
	// File and directory names
	agentConfigFile = "config/agent.toml"
	dataDir         = ".data"
	logsDir         = "logs"
	lockFile        = "agent.lock"

	// Gateway names
	gatewayCLI      = "cli"
	gatewayTelegram = "telegram"

	// Flags
	flagLog     = "--log"
	flagVerbose = "-v"
	flagDebug   = "--debug"
)

var (
	// Default configuration values
	defaultGateways = []string{gatewayCLI}

	// Shutdown signals
	shutdownSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}
)

// HandleServe starts the long-running daemon: daemon-hosted gateways
// (telegram) and the routines scheduler. `serve` is the primary name;
// `run`/`up` are kept as hidden aliases in main.go's dispatch, all calling
// this same function.
func HandleServe(args []string) error {
	// Parse flags
	var (
		path    = "."
		log     = false
		verbose = false
		debug   = false
	)

	// Manual flag parsing
	positional := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case flagLog:
			log = true
		case flagVerbose, "--verbose":
			verbose = true
		case flagDebug:
			debug = true
		default:
			positional = append(positional, arg)
		}
	}

	if len(positional) > 0 {
		path = positional[0]
	}

	// Resolve agent path
	absPath, err := registry.ResolveAgentPath(path)
	if err != nil {
		return err
	}

	// Register agent to keep registry current
	if err := registry.Register(absPath); err != nil {
		return fmt.Errorf("failed to register agent: %w", err)
	}

	// Load config and collect validation issues
	var allIssues []config.ValidationIssue

	agentCfg, agentIssues := config.LoadAgent(absPath)
	allIssues = append(allIssues, agentIssues...)

	// Load tools (only if agent config was loaded successfully)
	var tools []config.ToolConfig
	if agentCfg != nil {
		var toolIssues []config.ValidationIssue
		tools, toolIssues = config.LoadTools(absPath, agentCfg.Agent.Tools)
		allIssues = append(allIssues, toolIssues...)
	}

	// Load prompt for validation (minimal context - only syntax check)
	_, promptIssues := config.Parse(absPath, resolution.NewValidationContext(absPath))
	allIssues = append(allIssues, promptIssues...)

	// Load routines for validation. Unlike tools' "malformed file just
	// doesn't load" philosophy, a broken routine hard-fails startup here -
	// same as every other issue in allIssues below - since the blast radius
	// (a routine typo silently preventing the agent from starting at all)
	// is worse than letting it through.
	_, routineIssues := config.LoadRoutines(absPath)
	allIssues = append(allIssues, routineIssues...)

	// agentCfg must be loaded before we can build a logger or header (needs
	// provider/model/logging settings); if it's nil, config loading itself
	// failed, so print raw and bail — there's no logger to route through yet.
	if agentCfg == nil {
		for _, issue := range allIssues {
			fmt.Printf("[%s] %s\n", issue.Type, issue.Message)
		}
		return fmt.Errorf("failed to load agent configuration")
	}

	// Default gateways to ["cli"] if not specified
	gatewayNames := agentCfg.Access.Gateways
	if len(gatewayNames) == 0 {
		gatewayNames = defaultGateways
	}

	// Setup logger
	// logging = false → stdout only
	// logging = true or flags → stdout + file
	enableFileLogging := agentCfg.Agent.LoggingEnabled() || log || verbose || debug
	logDir := filepath.Join(absPath, dataDir, logsDir)
	lg, err := logger.Setup(logDir, verbose, debug, enableFileLogging)
	if err != nil {
		return fmt.Errorf("setup logger: %w", err)
	}

	logger.PrintBox([]string{
		fmt.Sprintf("agentctl · %s", filepath.Base(absPath)),
		fmt.Sprintf("%s/%s", agentCfg.Agent.Provider, agentCfg.Agent.Model),
		fmt.Sprintf("%d tools · %s", len(tools), strings.Join(gatewayNames, ", ")),
	})

	// Route validation issues through the logger now that one exists, so
	// they get [WARN]/[ERROR] tags and land in agent.log, not just stdout.
	hasErrors := false
	for _, issue := range allIssues {
		if issue.Type == config.IssueError {
			lg.Error(issue.Message)
			hasErrors = true
		} else {
			lg.Warn(issue.Message)
		}
	}
	if hasErrors {
		return fmt.Errorf("configuration validation failed")
	}

	lg.Info("agent started",
		"provider", agentCfg.Agent.Provider,
		"model", agentCfg.Agent.Model,
		"gateways", agentCfg.Access.Gateways,
	)

	// Ensure .data directory exists
	if err := os.MkdirAll(filepath.Join(absPath, dataDir), 0755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	// Acquire an exclusive lock to prevent two `serve` processes for the same
	// agent. The kernel releases this automatically on process exit (incl.
	// crash/kill -9), so no manual cleanup is needed.
	lockPath := filepath.Join(absPath, dataDir, lockFile)
	lockFd, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("open lock file: %w", err)
	}
	defer lockFd.Close()

	if err := syscall.Flock(int(lockFd.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("agent %q is already running (lock held on %s)", filepath.Base(absPath), lockPath)
	}

	// Record our PID so `agentctl stop` can find and signal this process.
	// The lock itself (not this PID) is the source of truth for "is it
	// running" - this is only used to know who to signal.
	if err := lockFd.Truncate(0); err != nil {
		return fmt.Errorf("write pid to lock file: %w", err)
	}
	if _, err := lockFd.WriteAt(fmt.Appendf(nil, "%d\n", os.Getpid()), 0); err != nil {
		return fmt.Errorf("write pid to lock file: %w", err)
	}

	// Migrate session files from unlinked contact folders to named identities
	if err := session.MigrateOnStartup(absPath); err != nil {
		return fmt.Errorf("session migration failed: %w", err)
	}

	// Warn about duplicate contacts in identities (first match wins in LookupPlatformID)
	if err := session.WarnDuplicateContacts(absPath, lg); err != nil {
		// Non-fatal - log warning and continue
		lg.Warn("failed to check for duplicate contacts", "error", err)
	}

	// Create audio transcriber if configured
	var transcriber audio.Transcriber
	if agentCfg.Audio != nil {
		var err error
		transcriber, err = audio.New(agentCfg.Audio, absPath)
		if err != nil {
			return fmt.Errorf("failed to initialize audio transcriber: %w", err)
		}
		lg.Info("audio transcriber initialized", "provider", agentCfg.Audio.Provider)
	}

	// Build the store, Orchestrator, and OutboundDispatcher, and register a
	// Sender for each requested gateway
	assembled, err := assembleOrchestrator(absPath, transcriber, lg, verbose, debug, gatewayNames)
	if err != nil {
		return err
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, shutdownSignals...)
	go func() {
		<-sigChan
		fmt.Println("\nShutting down...")
		cancel()
	}()

	// Start the routines scheduler (no-op if routines/ doesn't exist or is
	// empty). Runs for the lifetime of ctx, same as the gateway goroutines
	// below.
	routines.Start(ctx, absPath, agentCfg.Environment, lg, debug)

	// Start gateways
	errChan := make(chan error, len(assembled.Gateways))
	for i, gw := range assembled.Gateways {
		gwName := assembled.GatewayNames[i]
		go func(gwName string, gw internal.Gateway) {
			if err := gw.Start(ctx); err != nil {
				errChan <- fmt.Errorf("%s gateway error: %w", gwName, err)
			}
		}(gwName, gw)
	}

	// Wait for shutdown or error
	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return nil
	}
}
