package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agentmemory/agentmemory/internal/service"
)

// testObsSvc returns a minimal ObservationService for tests that validate
// request fields without calling RecordObservation. The service has no DB
// pool, so calling RecordObservation on it will fail — this is safe only
// for tests that exercise validation paths before the RecordObservation call.
func testObsSvc() *service.ObservationService {
	return service.NewObservationService(nil, nil)
}

// TestHandleCommitSession_NilService verifies that HandleCommitSession returns
// 503 Service Unavailable when the observation service is nil.
func TestHandleCommitSession_NilService(t *testing.T) {
	h := &RESTHandler{obsSvc: nil}

	body := `{"session_id":"sess-1","sha":"abc123def456","branch":"main","message":"test commit"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/api/session/commit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleCommitSession(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("HandleCommitSession with nil obsSvc returned status %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	// Verify the error response is valid JSON with expected fields
	var decoded map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("response body is not valid JSON: %v", err)
	}

	if decoded["error"] != "observation service not configured" {
		t.Errorf("error message = %q, want %q", decoded["error"], "observation service not configured")
	}
	if decoded["code"] != "SERVICE_UNAVAILABLE" {
		t.Errorf("error code = %q, want %q", decoded["code"], "SERVICE_UNAVAILABLE")
	}
}

// TestHandleCommitSession_Validation verifies that HandleCommitSession returns
// 400 for missing required fields.
func TestHandleCommitSession_Validation(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "invalid json",
			body:    `{not valid json`,
			wantErr: "invalid JSON body",
		},
		{
			name:    "empty body",
			body:    ``,
			wantErr: "invalid JSON body",
		},
		{
			name:    "missing session_id",
			body:    `{"sha":"abc123","branch":"main","message":"test"}`,
			wantErr: "session_id is required",
		},
		{
			name:    "missing sha",
			body:    `{"session_id":"sess-1","branch":"main","message":"test"}`,
			wantErr: "sha is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := &RESTHandler{obsSvc: testObsSvc()}

			req := httptest.NewRequest(http.MethodPost, "/v1/api/session/commit", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			h.HandleCommitSession(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}

			var decoded map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
				t.Fatalf("response body is not valid JSON: %v", err)
			}
			if decoded["error"] != tc.wantErr {
				t.Errorf("error message = %q, want %q", decoded["error"], tc.wantErr)
			}
		})
	}
}
