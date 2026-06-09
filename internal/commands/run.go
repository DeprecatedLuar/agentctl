package commands

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/DeprecatedLuar/agentctl/internal/config"
	"github.com/DeprecatedLuar/agentctl/internal/interfaces"
	"github.com/DeprecatedLuar/agentctl/internal/logger"
	"github.com/DeprecatedLuar/agentctl/internal/memory"
	_ "github.com/mattn/go-sqlite3"
)

const (
	// File and directory names
	agentConfigFile = "agent.toml"
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

	// Resolve to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Validate agent folder
	agentTomlPath := filepath.Join(absPath, agentConfigFile)
	if _, err := os.Stat(agentTomlPath); err != nil {
		return fmt.Errorf("not an agent folder (%s not found)", agentConfigFile)
	}

	// Load config
	agentCfg, err := config.LoadAgent(absPath)
	if err != nil {
		return err
	}

	// Load tools
	tools, err := config.LoadTools(absPath, agentCfg.Tools)
	if err != nil {
		return err
	}

	// Load prompt
	prompt, err := config.Parse(absPath, map[string]string{})
	if err != nil {
		return fmt.Errorf("failed to load prompt: %w", err)
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

	// Open database
	dbPath := filepath.Join(absPath, dataDir, "memory.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	// Initialize schema
	if err := memory.InitDB(db); err != nil {
		return fmt.Errorf("initialize database: %w", err)
	}

	// Create runner
	runner := &interfaces.Runner{
		AgentFolder: absPath,
		Config:      agentCfg,
		Tools:       tools,
		Prompt:      prompt,
		DB:          db,
		Logger:      lg,
		Verbose:     verbose,
		Debug:       debug,
	}

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

	// Start interfaces
	errChan := make(chan error, len(interfacesList))
	for _, iface := range interfacesList {
		switch iface {
		case interfaceCLI:
			cli := interfaces.NewCLI(absPath)
			go func() {
				if err := cli.Start(ctx, runner); err != nil {
					errChan <- fmt.Errorf("%s interface error: %w", interfaceCLI, err)
				}
			}()
		case interfaceTelegram:
			telegram := interfaces.NewTelegram(absPath)
			go func() {
				if err := telegram.Start(ctx, runner); err != nil {
					errChan <- fmt.Errorf("%s interface error: %w", interfaceTelegram, err)
				}
			}()
		default:
			return fmt.Errorf("unknown interface: %s", iface)
		}
	}

	// Wait for shutdown or error
	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return nil
	}
}
