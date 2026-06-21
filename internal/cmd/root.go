package cmd

import (
	"github.com/spf13/cobra"
)

// NewRootCommand creates the root agentmemory CLI command with all subcommands.
func NewRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "agentmemory",
		Short: "AgentMemory v2 — persistent memory for coding agents",
		Long: `AgentMemory v2 is a memory server for coding agents.
It captures observations, consolidates knowledge, and provides hybrid search.

Subcommands:
  serve    Start the HTTP/MCP server
  setup    Initialize database schema and extensions
  migrate  Apply pending database migrations
  user     Manage users
  connect  Configure a host coding agent to use AgentMemory
  team     Manage teams`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Register all subcommands
	rootCmd.AddCommand(NewServeCommand())
	rootCmd.AddCommand(NewSetupCommand())
	rootCmd.AddCommand(NewMigrateCommand())
	rootCmd.AddCommand(NewUserCommand())
	rootCmd.AddCommand(NewConnectCommand())
	rootCmd.AddCommand(NewTeamCommand())

	return rootCmd
}
