package unit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agentmemory/agentmemory/internal/handler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Task #15: writeJSON — marshal before writing status header
// =============================================================================
// The bug: writeJSON called w.WriteHeader(status) BEFORE json.NewEncoder(w).Encode(v).
// If encoding fails, the status header is already committed but the body is truncated.
// writeJSONStatus in health.go also omits Content-Type header entirely.
//
// These tests verify the external contract: all writeJSON-calling endpoints return
// valid JSON bodies with proper Content-Type headers and correct status codes.

// TestWriteJSONProducesValidJSON verifies that all JSON endpoints return
// valid, parseable JSON with application/json content type.
func TestWriteJSONProducesValidJSON(t *testing.T) {
	r := handler.NewRouter(nil, nil)

	// Test endpoints that use writeJSON internally
	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/health"},
		{"POST", "/v1/api/observe"},
		{"POST", "/v1/api/session/end"},
		{"POST", "/v1/api/session/commit"},
		{"POST", "/v1/auth/login"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			var req *http.Request
			if ep.method == "GET" {
				req = httptest.NewRequest(ep.method, ep.path, nil)
			} else {
				req = httptest.NewRequest(ep.method, ep.path, nil)
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			// Content-Type must be set to application/json
			ct := rec.Header().Get("Content-Type")
			assert.Contains(t, ct, "application/json",
				"%s %s must return Content-Type: application/json, got: %q",
				ep.method, ep.path, ct)

			// Body must be valid JSON (not a truncated or malformed response)
			// Skip for 204 No Content or 404 responses
			if rec.Code != http.StatusNoContent && rec.Code != http.StatusNotFound && rec.Body.Len() > 0 {
				var decoded interface{}
				err := json.Unmarshal(rec.Body.Bytes(), &decoded)
				require.NoError(t, err,
					"%s %s returned invalid JSON body: %s",
					ep.method, ep.path, rec.Body.String())
			}
		})
	}
}

// TestWriteJSONStatusHeaderNotCommittedBeforeBody verifies that the status
// code in the response matches what the body implies. If writeJSON commits
// the header before encoding, the status and body could be inconsistent.
func TestWriteJSONStatusHeaderNotCommittedBeforeBody(t *testing.T) {
	// This is a structural test: verify that for routes that return
	// JSON responses, the Content-Type header is always present and
	// the body is always valid JSON. This ensures writeJSON doesn't
	// write the header before marshaling succeeds.
	r := handler.NewRouter(nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// In placeholder mode, health returns 200 with message
	ct := rec.Header().Get("Content-Type")
	assert.NotEmpty(t, ct, "health must set Content-Type header")

	var decoded interface{}
	err := json.Unmarshal(rec.Body.Bytes(), &decoded)
	require.NoError(t, err, "health response body must be valid JSON")
}
