package handler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter creates and configures the chi HTTP router with middleware and route groups.
func NewRouter() chi.Router {
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

	// Auth middleware placeholder — returns 401 if no Authorization header
	authMw := stubAuthMiddleware

	// Authenticated route groups
	r.Group(func(r chi.Router) {
		r.Use(authMw)

		// Root (authenticated)
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"service":"agentmemory-v2","version":"2.0.0"}`))
		})

		// API routes
		r.Route("/v1/api", func(r chi.Router) {
			r.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"message":"API endpoint placeholder"}`))
			})
		})

		// MCP route
		r.Get("/v1/mcp", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"message":"MCP endpoint placeholder"}`))
		})

		// Socket/WebSocket route
		r.Get("/v1/socket", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"message":"Socket endpoint placeholder"}`))
		})
	})

	// Auth routes — mixed (some public, some authenticated)
	r.Route("/v1/auth", func(r chi.Router) {
		// Public auth endpoints (login, register)
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

		// Authenticated auth endpoints
		r.Group(func(r chi.Router) {
			r.Use(authMw)
			r.Get("/me", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"message":"profile placeholder"}`))
			})
		})
	})

	return r
}

// stubAuthMiddleware is a placeholder authentication middleware.
// It returns 401 Unauthorized for any request without an Authorization header.
// This will be replaced with real JWT/API-key authentication in a future phase.
func stubAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if token == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized","message":"authentication required"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
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
