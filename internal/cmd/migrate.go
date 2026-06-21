package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/spf13/cobra"
)

// NewMigrateCommand creates the `agentmemory migrate` command.
func NewMigrateCommand() *cobra.Command {
	var dbURL string

	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Apply pending database migrations",
		Long: `Apply all pending database migrations in order.

Reads migration files from the ./migrations directory.
Use --db-url to specify the database connection string.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dbURL == "" {
				dbURL = os.Getenv("DB_URL")
			}
			if dbURL == "" {
				return fmt.Errorf("DB_URL is required; set via --db-url flag or DB_URL environment variable")
			}

			return runMigrations(dbURL)
		},
	}

	cmd.Flags().StringVar(&dbURL, "db-url", "", "Database URL (or set DB_URL env var)")

	return cmd
}

// runMigrations applies all pending migrations from the ./migrations directory.
func runMigrations(dbURL string) error {
	m, err := migrate.New("file://migrations", dbURL)
	if err != nil {
		return fmt.Errorf("failed to initialize migrator: %w", err)
	}
	defer m.Close()

	// Check current version and dirty state
	version, dirty, err := m.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("failed to get migration version: %w", err)
	}

	if err == migrate.ErrNilVersion {
		slog.Info("no migrations applied yet (nil version)")
	} else {
		slog.Info("current migration state",
			"version", version,
			"dirty", dirty,
		)
	}

	if dirty {
		return fmt.Errorf("database is in a dirty state at version %d; manual intervention required", version)
	}

	// Apply pending migrations
	if err := m.Up(); err != nil {
		if err == migrate.ErrNoChange {
			slog.Info("no pending migrations — database is up to date")
			fmt.Println("No pending migrations — database is up to date.")
			return nil
		}
		return fmt.Errorf("migration failed: %w", err)
	}

	// Get new version
	newVersion, newDirty, err := m.Version()
	if err != nil {
		return fmt.Errorf("migration applied but failed to get new version: %w", err)
	}

	slog.Info("migrations applied successfully",
		"new_version", newVersion,
	)
	fmt.Printf("Migrations applied successfully.\n")
	fmt.Printf("  Version: %d\n", newVersion)
	fmt.Printf("  Dirty:   %v\n", newDirty)

	return nil
}
