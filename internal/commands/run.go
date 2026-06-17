package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/DeprecatedLuar/agentctl/internal"
	"github.com/DeprecatedLuar/agentctl/internal/config"
	"github.com/DeprecatedLuar/agentctl/internal/interfaces"
	"github.com/DeprecatedLuar/agentctl/internal/logger"
	"github.com/DeprecatedLuar/agentctl/internal/providers/audio"
	"github.com/DeprecatedLuar/agentctl/internal/registry"
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
		tools, toolIssues = config.LoadTools(absPath, agentCfg.Tools)
		allIssues = append(allIssues, toolIssues...)
	}

	// Load prompt for validation
	_, promptIssues := config.Parse(absPath, map[string]string{})
	allIssues = append(allIssues, promptIssues...)

	// Print validation results
	if len(allIssues) > 0 {
		hasErrors := false
		for _, issue := range allIssues {
			fmt.Printf("[%s] %s\n", issue.Type, issue.Message)
			if issue.Type == config.IssueError {
				hasErrors = true
			}
		}
		fmt.Println() // blank line after issues

		// Exit if any errors
		if hasErrors {
			return fmt.Errorf("configuration validation failed")
		}
	}

	// Ensure agentCfg is not nil at this point
	if agentCfg == nil {
		return fmt.Errorf("failed to load agent configuration")
	}

	// Setup logger
	// logging = false → stdout only
	// logging = true or flags → stdout + file
	enableFileLogging := agentCfg.IsLoggingEnabled() || log || verbose || debug
	logDir := filepath.Join(absPath, dataDir, logsDir)
	lg, err := logger.Setup(logDir, verbose, debug, enableFileLogging)
	if err != nil {
		return fmt.Errorf("setup logger: %w", err)
	}
	lg.Info("agent started",
		"provider", agentCfg.Provider,
		"model", agentCfg.Model,
		"interfaces", agentCfg.Interfaces,
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

	// Default interfaces to ["cli"] if not specified
	interfacesList := agentCfg.Interfaces
	if len(interfacesList) == 0 {
		interfacesList = defaultInterfaces
	}

	fmt.Printf("Agent: %s/%s\n", agentCfg.Provider, agentCfg.Model)
	fmt.Printf("Tools: %d | Interfaces: %v\n\n", len(tools), interfacesList)

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
			cli := interfaces.NewCLI(absPath, orch, store, lg, verbose)
			dispatcher.Register(cli)
			interfaceInstances = append(interfaceInstances, cli)
		case interfaceTelegram:
			telegram, err := interfaces.NewTelegram(absPath, transcriber, orch, lg, verbose)
			if err != nil {
				return fmt.Errorf("failed to initialize telegram interface: %w", err)
			}
			dispatcher.Register(telegram)
			interfaceInstances = append(interfaceInstances, telegram)
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
