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
)

func HandleRun(args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	// Resolve to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Validate agent folder
	agentTomlPath := filepath.Join(absPath, "agent.toml")
	if _, err := os.Stat(agentTomlPath); err != nil {
		return fmt.Errorf("not an agent folder (agent.toml not found)")
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

	// Open database (stub for now, will be implemented in Phase 5)
	var db *sql.DB
	// TODO: Open SQLite database in Phase 5

	// Create runner
	runner := &interfaces.Runner{
		AgentFolder: absPath,
		Config:      agentCfg,
		Tools:       tools,
		Prompt:      prompt,
		DB:          db,
	}

	// Default interfaces to ["cli"] if not specified
	interfacesList := agentCfg.Interfaces
	if len(interfacesList) == 0 {
		interfacesList = []string{"cli"}
	}

	fmt.Printf("Agent: %s/%s\n", agentCfg.Provider, agentCfg.Model)
	fmt.Printf("Tools: %d | Interfaces: %v\n\n", len(tools), interfacesList)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nShutting down...")
		cancel()
	}()

	// Start interfaces
	errChan := make(chan error, len(interfacesList))
	for _, iface := range interfacesList {
		switch iface {
		case "cli":
			cli := interfaces.NewCLI(absPath)
			go func() {
				if err := cli.Start(ctx, runner); err != nil {
					errChan <- fmt.Errorf("cli interface error: %w", err)
				}
			}()
		case "telegram":
			telegram := interfaces.NewTelegram(absPath)
			go func() {
				if err := telegram.Start(ctx, runner); err != nil {
					errChan <- fmt.Errorf("telegram interface error: %w", err)
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
