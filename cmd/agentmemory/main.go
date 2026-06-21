package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/agentmemory/agentmemory/internal/cmd"
	"github.com/agentmemory/agentmemory/internal/config"
)

func main() {
	// Initialize structured logging before anything else
	config.InitLogging()

	// Build the root cobra command
	rootCmd := cmd.NewRootCommand()

	// Execute the CLI
	if err := rootCmd.Execute(); err != nil {
		slog.Error("command failed", "error", err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
