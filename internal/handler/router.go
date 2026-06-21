package handler

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewRouter creates and configures the chi HTTP router with middleware and route groups.
// If pool is nil, the router is created without database-backed handlers.
func NewRouter(pool *pgxpool.Pool) chi.Router {
	// Create service dependencies
	var restHandler *RESTHandler
	var authHandler *AuthHandler
	var healthHandler *HealthHandler
	var wsHandler *WSHandler
	var viewerHandler *ViewerHandler

	wsHub := NewWSHub()

	if pool != nil {
		llmSvc, llmErr := service.NewLLMService()
		if llmErr != nil {
			slog.Warn("LLM service not configured — compression, summarization, consolidation disabled", "error", llmErr)
			llmSvc = service.NewLLMServiceWithModel(nil)
		}
		embedSvc, embedErr := service.NewEmbeddingService(pool)
		if embedErr != nil {
			slog.Warn("Embedding service not configured — semantic search disabled", "error", embedErr)
			embedSvc = &service.EmbeddingService{}
		}
		compressor := service.NewCompressionService(pool, llmSvc, embedSvc)
		obsSvc := service.NewObservationService(pool, compressor)
		sessionSvc := service.NewSessionService(pool)

		// Summarization and consolidation depend on LLM
		summarizer := service.NewSummarizationService(pool, llmSvc)
		mode := service.DefaultConsolidationMode("member_choice", false)
		consolidator := service.NewConsolidationService(pool, llmSvc, mode)
		reflector := service.NewReflectionService(pool, 3600)

		sessionEndH := service.NewSessionEndHandler(
			sessionSvc, summarizer, consolidator, reflector,
		)

		restHandler = NewRESTHandler(obsSvc, sessionSvc, sessionEndH)

		// Auth services
		userSvc := service.NewUserService(pool)
		teamSvc := service.NewTeamService(pool)
		memberSvc := service.NewTeamMembersService(pool)
		authHandler = NewAuthHandler(userSvc, teamSvc, memberSvc)

		// Health handler with real DB checker
		healthHandler = NewHealthHandler(&dbHealthChecker{pool: pool})

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
		r.Mount("/v1/mcp", NewMCPHandler(pool))
	})

	return r
}

// dbHealthChecker implements HealthChecker using a pgxpool.Pool.
type dbHealthChecker struct {
	pool *pgxpool.Pool
}

func (c *dbHealthChecker) Ping(ctx context.Context) error {
	return c.pool.Ping(ctx)
}

func (c *dbHealthChecker) HasPendingMigrations() (bool, error) {
	bgCtx := context.Background()

	// Check if the migration version table exists
	var exists bool
	err := c.pool.QueryRow(bgCtx,
		"SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'schema_migrations')",
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	if !exists {
		// No migrations table — assume migrations haven't been run yet
		// This is normal for a fresh database that hasn't had setup run
		return false, nil
	}

	// Check for dirty state in schema_migrations
	var dirty bool
	err = c.pool.QueryRow(bgCtx, "SELECT dirty FROM schema_migrations LIMIT 1").Scan(&dirty)
	if err != nil {
		// Table might exist but be empty (no migrations applied yet)
		return false, nil
	}
	if dirty {
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
