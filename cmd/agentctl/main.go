package main

import (
	"fmt"
	"os"

	"github.com/DeprecatedLuar/agentctl/internal/commands"
)

func main() {
	if len(os.Args) < 2 {
		printHelp()
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
	default:
		printHelp()
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println("agentctl - agentic workflow tool")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  agentctl init [path]    Initialize agent folder")
	fmt.Println("  agentctl run [path]     Run agent")
}
