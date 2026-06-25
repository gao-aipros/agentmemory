package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/agentmemory/agentmemory/internal/config"
	"github.com/agentmemory/agentmemory/internal/handler"
	"github.com/agentmemory/agentmemory/internal/mcp"
	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/spf13/cobra"
)

// NewServeCommand creates the `agentmemory serve` command.
func NewServeCommand() *cobra.Command {
	var (
		dbURL             string
		port              int
		migrateOnStartup  bool
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the AgentMemory HTTP/MCP server",
		Long: `Start the AgentMemory v2 server with HTTP REST API, MCP, and WebSocket support.

Requires a PostgreSQL/ParadeDB database. Set DB_URL or use --db-url.
Use --migrate-on-startup to auto-apply pending migrations.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Initialize structured logging
			config.InitLogging()

			// Resolve DB URL: flag > env
			if dbURL == "" {
				dbURL = os.Getenv("DB_URL")
			}
			if dbURL == "" {
				return fmt.Errorf("DB_URL is required; set via --db-url flag or DB_URL environment variable")
			}

			// Load full configuration from environment
			cfg := config.Load()

			// Resolve port: flag > env > default
			if port == 0 {
				port = cfg.Port
			}

			// Validate JWT secret is configured (Task #6 — fail loudly)
			if cfg.JWTSecret == "" {
				return fmt.Errorf("JWT_SECRET environment variable is required; set it to a strong random secret")
			}

			// Resolve migrate-on-startup: flag > env
			if !migrateOnStartup {
				migrateOnStartup = os.Getenv("MIGRATE_ON_STARTUP") == "true"
			}

			slog.Info("AgentMemory v2 starting",
				"port", port,
				"migrate_on_startup", migrateOnStartup,
			)

			// Run migrations if configured
			if migrateOnStartup {
				slog.Info("running migrations on startup")
				if err := runMigrations(dbURL); err != nil {
					return fmt.Errorf("migration failed: %w", err)
				}
				slog.Info("migrations completed")
			}

			// Create database connection pool
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			pool, err := config.NewPool(ctx, dbURL)
			if err != nil {
				slog.Error("failed to create database pool", "error", err)
				return fmt.Errorf("database connection failed: %w", err)
			}
			defer config.ClosePool(pool)

			slog.Info("database connection pool created")

			// Create shared ServiceBundle once — both REST router and MCP handler
			// use the same service instances, avoiding duplicate wiring.
			bundle := mcp.NewServiceBundle(pool)

			// Start pipeline scheduler (in-process, goroutine-based)
			schedulerCtx, schedulerCancel := context.WithCancel(context.Background())
			defer schedulerCancel()
			scheduler := service.NewScheduler(pool, bundle.LLM, bundle.Embedding, service.SchedulerIntervals{
				Compression:   cfg.CompressionInterval,
				Summarization: cfg.SummarizationInterval,
				Consolidation: cfg.ConsolidationInterval,
				Reflection:    cfg.ReflectionInterval,
			})
			scheduler.Start(schedulerCtx)

			// Create HTTP router with the shared bundle and config
			router := handler.NewRouter(bundle, cfg)

			// Replace placeholder health check with real one
			// Note: NewRouter sets a placeholder; we override it here.
			// The router setup in handler.NewRouter sets a simple health check;
			// the real health handler is wired via handler.NewHealthHandler when pool != nil

			// Configure HTTP server
			addr := fmt.Sprintf(":%d", port)
			srv := &http.Server{
				Addr:         addr,
				Handler:      router,
				ReadTimeout:  15 * time.Second,
				WriteTimeout: 15 * time.Second,
				IdleTimeout:  60 * time.Second,
			}

			// Print startup banner
			fmt.Printf("\n")
			fmt.Printf("  ╔══════════════════════════════════════════╗\n")
			fmt.Printf("  ║        AgentMemory v2.0.0                ║\n")
			fmt.Printf("  ║        Persistent Memory for Agents      ║\n")
			fmt.Printf("  ╠══════════════════════════════════════════╣\n")
			fmt.Printf("  ║  HTTP API:  http://localhost%-12d  ║\n", port)
			fmt.Printf("  ║  MCP:       http://localhost:%d/v1/mcp ║\n", port)
			fmt.Printf("  ║  Health:    http://localhost:%d/health ║\n", port)
			fmt.Printf("  ╚══════════════════════════════════════════╝\n")
			fmt.Printf("\n")

			// Start server in a goroutine
			errCh := make(chan error, 1)
			go func() {
				slog.Info("HTTP server listening", "addr", addr)
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					errCh <- err
				}
			}()

			// Wait for interrupt signal or server error
			quit := make(chan os.Signal, 1)
			signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

			select {
			case sig := <-quit:
				slog.Info("received shutdown signal", "signal", sig.String())
			case err := <-errCh:
				slog.Error("server error", "error", err)
				return err
			}

			// Graceful shutdown with timeout
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer shutdownCancel()

			slog.Info("shutting down HTTP server")
			if err := srv.Shutdown(shutdownCtx); err != nil {
				slog.Error("HTTP server shutdown error", "error", err)
			}

			// Wait for in-flight pipeline goroutines to drain before closing
			// the database connection pool.
			slog.Info("waiting for memory pipeline goroutines to drain")
			bundle.SessionEnd.Wait()

			slog.Info("AgentMemory v2 stopped")
			return nil
		},
	}

	cmd.Flags().StringVar(&dbURL, "db-url", "", "Database URL (or set DB_URL env var)")
	cmd.Flags().IntVar(&port, "port", 0, "Server port (or set PORT env var, default 8080)")
	cmd.Flags().BoolVar(&migrateOnStartup, "migrate-on-startup", false, "Run migrations on startup (or set MIGRATE_ON_STARTUP=true)")

	return cmd
}
