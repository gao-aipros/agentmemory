package main

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
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	// Initialize structured logging
	config.InitLogging()

	// Load configuration from environment
	cfg := config.Load()

	slog.Info("AgentMemory v2 starting",
		"port", cfg.Port,
		"log_level", cfg.LogLevel,
	)

	// Create database connection pool (temporary — will be wired properly in US5)
	var pool *pgxpool.Pool
	if cfg.DBURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var err error
		pool, err = config.NewPool(ctx, cfg.DBURL)
		if err != nil {
			slog.Error("failed to create database pool", "error", err)
			os.Exit(1)
		}
		defer config.ClosePool(pool)
		slog.Info("database connection pool created",
			"max_conns", cfg.DBMaxConns,
			"min_conns", cfg.DBMinConns,
		)
	} else {
		slog.Warn("DB_URL not set — running without database connection")
	}

	// Create HTTP router
	router := handler.NewRouter(pool)

	// Configure HTTP server
	addr := fmt.Sprintf(":%d", cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine so we can listen for shutdown signals
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
	}

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	slog.Info("shutting down HTTP server")
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}

	slog.Info("AgentMemory v2 stopped")
}
