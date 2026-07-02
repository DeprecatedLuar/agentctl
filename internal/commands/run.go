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
	"github.com/DeprecatedLuar/agentctl/internal/interfaces"
	"github.com/DeprecatedLuar/agentctl/internal/interfaces/cli"
	"github.com/DeprecatedLuar/agentctl/internal/interfaces/telegram"
	"github.com/DeprecatedLuar/agentctl/internal/logger"
	"github.com/DeprecatedLuar/agentctl/internal/providers/audio"
	"github.com/DeprecatedLuar/agentctl/internal/registry"
	"github.com/DeprecatedLuar/agentctl/internal/resolution"
	"github.com/DeprecatedLuar/agentctl/internal/session"
)

const (
	// File and directory names
	agentConfigFile = "config/agent.toml"
	dataDir         = ".data"
	logsDir         = "logs"

	// Interface names
	interfaceCLI      = "cli"
	interfaceTelegram = "telegram"

	// Flags
	flagLog     = "--log"
	flagVerbose = "-v"
	flagDebug   = "--debug"
)

var (
	// Default configuration values
	defaultInterfaces = []string{interfaceCLI}

	// Shutdown signals
	shutdownSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}
)

func HandleRun(args []string) error {
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

	// agentCfg must be loaded before we can build a logger or header (needs
	// provider/model/logging settings); if it's nil, config loading itself
	// failed, so print raw and bail — there's no logger to route through yet.
	if agentCfg == nil {
		for _, issue := range allIssues {
			fmt.Printf("[%s] %s\n", issue.Type, issue.Message)
		}
		return fmt.Errorf("failed to load agent configuration")
	}

	// Default interfaces to ["cli"] if not specified
	interfacesList := agentCfg.Access.Interfaces
	if len(interfacesList) == 0 {
		interfacesList = defaultInterfaces
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
		fmt.Sprintf("%d tools · %s", len(tools), strings.Join(interfacesList, ", ")),
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
		"interfaces", agentCfg.Access.Interfaces,
	)

	// Ensure .data directory exists
	if err := os.MkdirAll(filepath.Join(absPath, dataDir), 0755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
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

	// Create session store
	store := session.NewJSONLStore(absPath)

	// Create orchestrator (orchestration layer)
	orch := &internal.Orchestrator{
		AgentFolder:  absPath,
		SessionStore: store,
		Logger:       lg,
		Verbose:      verbose,
		Debug:        debug,
	}

	// Create outbound dispatcher
	dispatcher := interfaces.NewOutboundDispatcher()

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

	// Create and register interfaces
	var interfaceInstances []internal.Interface
	for _, iface := range interfacesList {
		switch iface {
		case interfaceCLI:
			cliInterface := cli.NewCLI(absPath, orch, store, lg, verbose)
			dispatcher.Register(cliInterface)
			interfaceInstances = append(interfaceInstances, cliInterface)
		case interfaceTelegram:
			telegramInterface, err := telegram.NewTelegram(absPath, transcriber, orch, store, lg, verbose)
			if err != nil {
				return fmt.Errorf("failed to initialize telegram interface: %w", err)
			}
			dispatcher.Register(telegramInterface)
			interfaceInstances = append(interfaceInstances, telegramInterface)
		default:
			return fmt.Errorf("unknown interface: %s", iface)
		}
	}

	// Assign dispatcher to orchestrator
	orch.Dispatcher = dispatcher

	// Start interfaces
	errChan := make(chan error, len(interfaceInstances))
	for i, iface := range interfaceInstances {
		ifaceName := interfacesList[i]
		go func(ifaceName string, iface internal.Interface) {
			if err := iface.Start(ctx); err != nil {
				errChan <- fmt.Errorf("%s interface error: %w", ifaceName, err)
			}
		}(ifaceName, iface)
	}

	// Wait for shutdown or error
	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return nil
	}
}
