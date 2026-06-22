package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
)

// HealthChecker defines the interface for checking database and migration health.
type HealthChecker interface {
	Ping(ctx context.Context) error
	HasPendingMigrations() (bool, error)
}

// HealthHandler handles the GET /health endpoint.
// It is public (no auth required) and returns the current health status.
type HealthHandler struct {
	checker HealthChecker
	version string
}

// NewHealthHandler creates a new HealthHandler with the given health checker.
func NewHealthHandler(checker HealthChecker) *HealthHandler {
	return &HealthHandler{
		checker: checker,
		version: "2.0.0",
	}
}

// healthResponse is the JSON response for the health endpoint.
type healthResponse struct {
	Status     string `json:"status"`
	DB         string `json:"db,omitempty"`
	Migrations string `json:"migrations,omitempty"`
	Version    string `json:"version,omitempty"`
}

// ServeHTTP implements http.Handler for the health endpoint.
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	resp := healthResponse{
		Version: h.version,
	}

	// Check database connectivity
	if h.checker != nil {
		if err := h.checker.Ping(r.Context()); err != nil {
			slog.Warn("health check: database ping failed", "error", err)
			resp.Status = "unhealthy"
			resp.DB = "disconnected"
			writeJSONStatus(w, http.StatusServiceUnavailable, resp)
			return
		}
		resp.DB = "connected"
	}

	// Check for pending migrations
	if h.checker != nil {
		pending, err := h.checker.HasPendingMigrations()
		if err != nil {
			// If we can't check migrations, log the error but don't fail health
			slog.Warn("health check: failed to check migrations", "error", err)
		} else if pending {
			resp.Status = "unhealthy"
			resp.Migrations = "pending"
			writeJSONStatus(w, http.StatusServiceUnavailable, resp)
			return
		}
	}

	// All checks passed
	resp.Status = "ok"
	writeJSONStatus(w, http.StatusOK, resp)
}

// writeJSONStatus writes a JSON response with the given status code.
// Marshal happens BEFORE WriteHeader so a failed encode does not leave a
// committed status with a truncated body.
func writeJSONStatus(w http.ResponseWriter, status int, v interface{}) {
	buf, err := json.Marshal(v)
	if err != nil {
		slog.Error("failed to write JSON response", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"status":"error","error":"internal server error"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(buf)
}
