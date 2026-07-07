package main

import (
	"fmt"
	"os"

	"github.com/DeprecatedLuar/agentctl/internal/commands"
	"github.com/DeprecatedLuar/agentctl/internal/commands/help"
)

func main() {
	if len(os.Args) < 2 {
		help.HandleHelp(nil)
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "init":
		if err := commands.HandleInit(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "run":
		if err := commands.HandleRun(args); err != nil {
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
	case "help":
		if err := help.HandleHelp(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	default:
		help.HandleHelp(nil)
		os.Exit(1)
	}
}
