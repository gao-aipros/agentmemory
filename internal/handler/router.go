package handler

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/agentmemory/agentmemory/internal/config"
	"github.com/agentmemory/agentmemory/internal/mcp"
	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewRouter creates and configures the chi HTTP router with middleware and route groups.
// bundle is the shared ServiceBundle created once at startup by cmd/serve.
// If bundle or bundle.Pool is nil, the router is created without database-backed handlers.
func NewRouter(bundle *mcp.ServiceBundle, cfg *config.Config) chi.Router {
	// Create service dependencies from the shared bundle
	var restHandler *RESTHandler
	var authHandler *AuthHandler
	var healthHandler *HealthHandler
	var wsHandler *WSHandler
	var viewerHandler *ViewerHandler

	wsHub := NewWSHub()
	var pool *pgxpool.Pool

	if bundle != nil && bundle.Pool != nil {
		pool = bundle.Pool

		// Use services from the shared bundle (created once at startup)
		restHandler = NewRESTHandler(bundle.Observation, bundle.Session, bundle.SessionEnd)

		// Auth services
		authHandler = NewAuthHandler(cfg, bundle.User, bundle.Team, bundle.Members)

		// Health handler with real DB checker
		healthHandler = NewHealthHandler(&dbHealthChecker{pool: pool, queries: store.New(pool)})

		// WebSocket handler
		wsHandler = NewWSHandler(wsHub)
	}

	// Viewer handler (always available, even without DB)
	var err error
	viewerHandler, err = NewViewerHandler()
	if err != nil {
		slog.Warn("failed to initialize viewer handler", "error", err)
	}

	r := chi.NewRouter()

	// Global middleware — applied to ALL routes
	r.Use(SecurityHeadersMiddleware) // Security headers first (set on every response)
	r.Use(SanitizeInputMiddleware)   // Strip HTML tags from query parameters (XSS prevention)
	r.Use(RateLimitMiddleware)       // Rate limiting placeholder (no-op until implemented)
	r.Use(middleware.RequestID)
	r.Use(slogLoggerMiddleware)
	r.Use(middleware.Recoverer)

	// Health check — public, no auth required
	if healthHandler != nil {
		r.Get("/health", healthHandler.ServeHTTP)
	} else {
		r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok","version":"2.0.0"}`))
		})
	}

	// Auth routes — mixed (some public, some authenticated)
	r.Route("/v1/auth", func(r chi.Router) {
		// Public auth endpoints
		if authHandler != nil {
			r.Post("/login", authHandler.HandleLogin)
			r.Post("/register", authHandler.HandleRegister)
		} else {
			r.Post("/login", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"message":"login placeholder"}`))
			})
			r.Post("/register", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"message":"register placeholder"}`))
			})
		}

		// Authenticated auth endpoints
		r.Group(func(r chi.Router) {
			if pool != nil {
				r.Use(AuthMiddleware(pool))
			}
			if authHandler != nil {
				r.Get("/me", authHandler.HandleGetMe)
				r.Get("/keys", authHandler.HandleListAPIKeys)
				r.Post("/keys", authHandler.HandleCreateAPIKey)
				r.Delete("/keys/{key_id}", authHandler.HandleDeleteAPIKey)
			} else {
				r.Get("/me", func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"message":"profile placeholder"}`))
				})
			}
		})
	})

	// Authenticated route groups
	r.Group(func(r chi.Router) {
		if pool != nil {
			r.Use(AuthMiddleware(pool))
		}

		// Root and socket routes require session token (UI routes)
		r.Group(func(r chi.Router) {
			r.Use(RequireSessionToken)

			// SPA Viewer at /
			if viewerHandler != nil {
				r.Get("/", viewerHandler.ServeHTTP)
				r.Get("/*", viewerHandler.ServeHTTP)
			} else {
				r.Get("/", func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"service":"agentmemory-v2","version":"2.0.0"}`))
				})
			}

			// WebSocket endpoint
			if wsHandler != nil {
				r.Get("/v1/socket", wsHandler.ServeHTTP)
			} else {
				r.Get("/v1/socket", func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"message":"Socket endpoint placeholder"}`))
				})
			}
		})

		// API routes (allow both session tokens and API keys)
		r.Route("/v1/api", func(r chi.Router) {
			if restHandler != nil {
				r.Post("/observe", restHandler.HandleObserve)
				r.Post("/session/end", restHandler.HandleEndSession)
				r.Post("/session/commit", restHandler.HandleCommitSession)
			} else {
				r.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"message":"API endpoint placeholder — no database configured"}`))
				})
			}
		})

		// MCP route — StreamableHTTP handler supports both GET (SSE) and POST (JSON-RPC)
		r.Mount("/v1/mcp", NewMCPHandler(bundle))
	})

	return r
}

// dbHealthChecker implements HealthChecker using sqlc-generated store queries.
type dbHealthChecker struct {
	pool    *pgxpool.Pool
	queries *store.Queries
}

func (c *dbHealthChecker) Ping(ctx context.Context) error {
	return c.pool.Ping(ctx)
}

func (c *dbHealthChecker) HasPendingMigrations() (bool, error) {
	ctx := context.Background()

	// Check if the migration version table exists
	exists, err := c.queries.CheckSchemaMigrationsTableExists(ctx)
	if err != nil {
		return false, err
	}
	if !exists {
		// No migrations table — assume migrations haven't been run yet.
		// This is normal for a fresh database that hasn't had setup run.
		return false, nil
	}

	// Check for dirty state via sqlc-generated query
	migration, err := c.queries.GetMigrationVersion(ctx)
	if err != nil {
		// Table exists but is empty (no migrations applied yet) — this is fine
		return false, nil
	}
	if migration.Dirty {
		// Dirty state is a critical issue, not just pending
		return true, nil
	}

	// If we can't easily check pending count without golang-migrate's logic,
	// assume no pending migrations (the migrate command handles this)
	return false, nil
}

// slogLoggerMiddleware logs each HTTP request using the structured logger (slog).
// It records the method, path, status code, duration, and request ID.
func slogLoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap the response writer to capture the status code
		wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(wrapped, r)

		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", middleware.GetReqID(r.Context()),
			"remote_addr", r.RemoteAddr,
		)
	})
}
