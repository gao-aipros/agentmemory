package handler

import (
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
	if pool != nil {
		llmSvc := service.NewLLMService(nil) // No LLM provider configured by default
		embedSvc := service.NewEmbeddingService(pool, nil)
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
	}
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.RequestID)
	r.Use(slogLoggerMiddleware)
	r.Use(middleware.Recoverer)

	// Health check — no auth required
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

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

			r.Get("/", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"service":"agentmemory-v2","version":"2.0.0"}`))
			})

			// Socket/WebSocket route
			r.Get("/v1/socket", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"message":"Socket endpoint placeholder"}`))
			})
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
