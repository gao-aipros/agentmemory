package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/agentmemory/agentmemory/internal/config"
	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/spf13/cobra"
)

// NewSetupCommand creates the `agentmemory setup` command.
func NewSetupCommand() *cobra.Command {
	var dbURL string

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Initialize database schema, extensions, and run all migrations",
		Long: `Setup initializes the AgentMemory database by:
1. Enabling required PostgreSQL extensions (pg_search, vector)
2. Running all pending migrations

This is idempotent — safe to run multiple times.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dbURL == "" {
				dbURL = os.Getenv("DB_URL")
			}
			if dbURL == "" {
				return fmt.Errorf("DB_URL is required; set via --db-url flag or DB_URL environment variable")
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			// Create a connection pool for extension setup
			pool, err := config.NewPool(ctx, dbURL)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer config.ClosePool(pool)

			// Create store queries backed by the pool (sqlc-generated, no raw SQL)
			queries := store.New(pool)

			// Enable extensions using sqlc-generated queries
			if err := queries.CreatePgSearchExtension(ctx); err != nil {
				slog.Warn("failed to enable extension", "extension", "pg_search", "error", err)
				fmt.Printf("Warning: could not enable extension pg_search: %v\n", err)
			} else {
				slog.Info("extension enabled", "extension", "pg_search")
				fmt.Println("Extension enabled: pg_search")
			}
			if err := queries.CreateVectorExtension(ctx); err != nil {
				slog.Warn("failed to enable extension", "extension", "vector", "error", err)
				fmt.Printf("Warning: could not enable extension vector: %v\n", err)
			} else {
				slog.Info("extension enabled", "extension", "vector")
				fmt.Println("Extension enabled: vector")
			}

			// Run migrations
			fmt.Println("Running migrations...")
			if err := runMigrations(dbURL); err != nil {
				return fmt.Errorf("migration failed: %w", err)
			}

			// Verify tables were created by listing them via sqlc-generated query
			tables, err := queries.ListPublicTables(ctx)
			if err != nil {
				slog.Warn("failed to list tables", "error", err)
			} else {
				fmt.Printf("\nSetup complete. Tables created (%d):\n", len(tables))
				for _, t := range tables {
					fmt.Printf("  - %s\n", t)
				}
			}

			fmt.Println("\nSetup completed successfully.")
			return nil
		},
	}

	cmd.Flags().StringVar(&dbURL, "db-url", "", "Database URL (or set DB_URL env var)")

	return cmd
}
