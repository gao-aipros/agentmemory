package unit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agentmemory/agentmemory/internal/handler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T146: Health check logic tests.
// Verifies: DB alive -> 200, DB dead -> 503, migrations pending -> 503.

// mockHealthChecker implements the handler.HealthChecker interface for testing.
type mockHealthChecker struct {
	pingErr           error
	migrationsPending bool
	migrationErr      error
}

func (m *mockHealthChecker) Ping(ctx context.Context) error {
	return m.pingErr
}

func (m *mockHealthChecker) HasPendingMigrations() (bool, error) {
	return m.migrationsPending, m.migrationErr
}

// healthResponse matches the JSON response from the health endpoint.
type healthResponse struct {
	Status     string `json:"status"`
	DB         string `json:"db,omitempty"`
	Migrations string `json:"migrations,omitempty"`
	Version    string `json:"version,omitempty"`
}

// TestHealthCheckDBAlive verifies 200 when DB is healthy.
func TestHealthCheckDBAlive(t *testing.T) {
	checker := &mockHealthChecker{
		pingErr:           nil,
		migrationsPending: false,
	}

	healthHandler := handler.NewHealthHandler(checker)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	healthHandler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp healthResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "ok", resp.Status, "status should be ok when DB is alive")
	assert.Equal(t, "connected", resp.DB, "db should be connected")
	assert.Equal(t, "2.0.0", resp.Version, "version should be 2.0.0")
}

// TestHealthCheckDBDead verifies 503 when DB ping fails.
func TestHealthCheckDBDead(t *testing.T) {
	checker := &mockHealthChecker{
		pingErr:           assert.AnError,
		migrationsPending: false,
	}

	healthHandler := handler.NewHealthHandler(checker)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	healthHandler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var resp healthResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "unhealthy", resp.Status, "status should be unhealthy when DB is dead")
	assert.Equal(t, "disconnected", resp.DB, "db should be disconnected")
}

// TestHealthCheckMigrationsPending verifies 503 when migrations are pending.
func TestHealthCheckMigrationsPending(t *testing.T) {
	checker := &mockHealthChecker{
		pingErr:           nil,
		migrationsPending: true,
	}

	healthHandler := handler.NewHealthHandler(checker)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	healthHandler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var resp healthResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "unhealthy", resp.Status, "status should be unhealthy when migrations are pending")
	assert.Equal(t, "pending", resp.Migrations, "migrations should be pending")
}

// TestHealthCheckDBDeadWithPendingMigrations verifies DB dead takes priority.
func TestHealthCheckDBDeadWithPendingMigrations(t *testing.T) {
	checker := &mockHealthChecker{
		pingErr:           assert.AnError,
		migrationsPending: true,
	}

	healthHandler := handler.NewHealthHandler(checker)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	healthHandler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var resp healthResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "unhealthy", resp.Status)
	assert.Equal(t, "disconnected", resp.DB, "DB check should take priority over migrations")
}

// TestHealthCheckContentType verifies the health endpoint returns JSON content type.
func TestHealthCheckContentType(t *testing.T) {
	checker := &mockHealthChecker{
		pingErr:           nil,
		migrationsPending: false,
	}

	healthHandler := handler.NewHealthHandler(checker)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	healthHandler.ServeHTTP(rec, req)

	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
}

// TestHealthCheckPublicAccess verifies health endpoint does not require auth.
func TestHealthCheckPublicAccess(t *testing.T) {
	checker := &mockHealthChecker{
		pingErr:           nil,
		migrationsPending: false,
	}

	healthHandler := handler.NewHealthHandler(checker)

	// No Authorization header
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	healthHandler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, "health check should succeed without auth")
}

// TestHealthCheckMigrationError verifies error checking migrations is handled gracefully.
func TestHealthCheckMigrationError(t *testing.T) {
	checker := &mockHealthChecker{
		pingErr:           nil,
		migrationsPending: false,
		migrationErr:      assert.AnError,
	}

	healthHandler := handler.NewHealthHandler(checker)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	healthHandler.ServeHTTP(rec, req)

	// Migration check error should not break health; we assume no pending if error
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestHealthCheckResponseStructure verifies all expected fields are present.
func TestHealthCheckResponseStructure(t *testing.T) {
	checker := &mockHealthChecker{
		pingErr:           nil,
		migrationsPending: false,
	}

	healthHandler := handler.NewHealthHandler(checker)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	healthHandler.ServeHTTP(rec, req)

	var resp healthResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.NotEmpty(t, resp.Status, "status field should be present")
	assert.NotEmpty(t, resp.DB, "db field should be present")
	assert.NotEmpty(t, resp.Version, "version field should be present")
}
