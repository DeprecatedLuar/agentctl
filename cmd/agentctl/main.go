package main

import (
	"fmt"
	"os"

	"github.com/DeprecatedLuar/agentctl/internal/commands"
	"github.com/DeprecatedLuar/agentctl/internal/commands/help"
)

func main() {
	if len(os.Args) < 2 {
		if err := commands.HandleChat(nil); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "init":
		if err := commands.HandleInit(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "serve", "run", "up":
		// `serve` is the primary name; `run`/`up` are hidden aliases kept
		// for backward compatibility and muscle memory.
		if err := commands.HandleServe(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "stop":
		if err := commands.HandleStop(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "chat":
		if err := commands.HandleChat(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "getagent":
		if err := commands.HandleGetAgent(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "models":
		if err := commands.HandleModels(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "toolrun":
		if err := commands.HandleToolRun(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "inject":
		if err := commands.HandleInject(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "deliver":
		if err := commands.HandleDeliver(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "monitor":
		if err := commands.HandleMonitor(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "help":
		if err := help.HandleHelp(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	default:
		// Not a recognized subcommand: forward to chat as a shortcut,
		// e.g. `agentctl -m "hello"` == `agentctl chat -m "hello"`.
		// Pass the full tail so the unrecognized token itself (a flag
		// like -m/-a, or a typo) is what chat's parser sees first.
		if err := commands.HandleChat(os.Args[1:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}
}
