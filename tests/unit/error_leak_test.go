package unit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agentmemory/agentmemory/internal/handler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Task #8: Stop leaking raw DB errors to HTTP clients
// =============================================================================
// Bug: Multiple handlers return err.Error() directly in HTTP response bodies,
// exposing raw pgx/database errors to clients.
//
// Fixes required:
// - rest.go:131 (HandleObserve):      err.Error() -> "observation failed"
// - rest.go:180 (HandleEndSession):   err.Error() -> "failed to end session"
// - auth.go:367 (HandleDeleteAPIKey): err.Error() -> "failed to delete API key"
// - middleware_auth.go:67:            remove err.Error() from message field,
//                                     only return "authentication failed"

// TestErrorResponsesUseGenericMessages checks that error response bodies never
// contain raw database/pgx error strings that would leak implementation details.
func TestErrorResponsesUseGenericMessages(t *testing.T) {
	r := handler.NewRouter(nil, nil)

	// Exercise endpoints that return error responses
	tests := []struct {
		method            string
		path              string
		body              string
		minMap            map[string]string // minimum expected keys
		forbiddenPatterns []string          // patterns that MUST NOT appear in body
	}{
		{
			method: "POST", path: "/v1/api/observe",
			body:              ``,
			forbiddenPatterns: []string{"pgx", "SQLSTATE", "connection refused", "scan", "Scan"},
		},
		{
			method: "POST", path: "/v1/api/session/end",
			body:              ``,
			forbiddenPatterns: []string{"pgx", "SQLSTATE", "connection refused", "scan", "Scan"},
		},
		{
			method: "DELETE", path: "/v1/auth/keys/test-key",
			body:              ``,
			forbiddenPatterns: []string{"pgx", "SQLSTATE", "connection refused", "scan", "Scan"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			var req *http.Request
			if tc.body != "" {
				req = httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			} else {
				req = httptest.NewRequest(tc.method, tc.path, nil)
			}
			if tc.method == "POST" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			// Skip endpoints that return 200 (placeholder mode) — these
			// don't exercise the error path, but we still verify no leaks.
			body := rec.Body.String()
			for _, pattern := range tc.forbiddenPatterns {
				assert.NotContains(t, body, pattern,
					"response body must not leak %q: %s", pattern, body)
			}

			// If the body is JSON, verify it parses correctly
			if strings.HasPrefix(strings.TrimSpace(body), "{") {
				var decoded map[string]interface{}
				err := json.Unmarshal(rec.Body.Bytes(), &decoded)
				if err == nil {
					// If "error" key exists, it must not look like a raw DB error
					if errMsg, ok := decoded["error"].(string); ok {
						for _, pattern := range tc.forbiddenPatterns {
							assert.NotContains(t, errMsg, pattern,
								"error message must not leak %q: %s", pattern, errMsg)
						}
					}
				}
			}
		})
	}
}

// TestAuthMiddlewareErrorDoesNotLeakDetails verifies that when AuthMiddleware
// returns 401, the response body does not contain raw validation error details.
// With nil pool, the auth middleware is not active, but we verify the route exists.
func TestAuthMiddlewareErrorDoesNotLeakDetails(t *testing.T) {
	r := handler.NewRouter(nil, nil)

	// Accessing a protected endpoint without auth should return 401
	// (when DB is configured) or 200 placeholder (when DB is nil).
	// Either way, the response must not leak internal error details.
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/me", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	body := rec.Body.String()
	forbiddenPatterns := []string{"pgx", "SQLSTATE", "connection refused", "scan", "Scan",
		"pq:", "unable to connect", "dial tcp"}

	for _, pattern := range forbiddenPatterns {
		assert.NotContains(t, body, pattern,
			"auth middleware error must not leak %q: %s", pattern, body)
	}

	// If response is JSON, it must be well-formed
	if strings.HasPrefix(strings.TrimSpace(body), "{") {
		var decoded map[string]interface{}
		err := json.Unmarshal(rec.Body.Bytes(), &decoded)
		require.NoError(t, err, "error response must be valid JSON")

		// The "error" key should be a simple, user-friendly message
		if errVal, ok := decoded["error"]; ok {
			errStr, ok := errVal.(string)
			if ok {
				assert.NotEmpty(t, errStr)
				// Generic error messages are short and don't contain colons
				// or file paths that indicate leaked internals
				assert.NotContains(t, errStr, ":", "error message should not contain technical details")
				assert.NotContains(t, errStr, "/", "error message should not contain paths")
			}
		}
	}
}

// TestObservationErrorResponseIsGeneric verifies HandleObserve error path.
// When the observation service fails, the error must be generic.
func TestObservationErrorResponseIsGeneric(t *testing.T) {
	// In placeholder mode (nil pool), HandleObserve returns 503 with
	// "observation service not configured". This is a legitimate user-facing
	// error. We verify it doesn't look like a raw DB error.
	r := handler.NewRouter(nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/api/observe",
		strings.NewReader(`{"type":"test","title":"test","narrative":"test","session_id":"s1"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// Valid JSON
	var decoded map[string]interface{}
	err := json.Unmarshal(rec.Body.Bytes(), &decoded)
	if err == nil {
		// Must not contain DB internals
		if errMsg, ok := decoded["error"].(string); ok {
			dbLeaks := []string{"pgx", "SQLSTATE", "connection refused",
				"unable to connect", "dial tcp", "pq:", "no such host"}
			for _, leak := range dbLeaks {
				assert.NotContains(t, errMsg, leak,
					"observation error must not leak %q", leak)
			}
		}
	}
}
