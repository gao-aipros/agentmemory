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

// =============================================================================
// #45: Session/end response missing summary_queued and consolidation_queued
// =============================================================================

// TestEndSessionResponse_HasQueueFields verifies that endSessionResponse JSON
// includes summary_queued and consolidation_queued boolean fields.
func TestEndSessionResponse_HasQueueFields(t *testing.T) {
	resp := endSessionResponse{
		SessionID: "sess-1",
		Status:    "ended",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal endSessionResponse: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	// Verify summary_queued field exists
	sq, ok := decoded["summary_queued"]
	if !ok {
		t.Fatal("endSessionResponse JSON missing required field: summary_queued")
	}
	if _, isBool := sq.(bool); !isBool {
		t.Fatalf("summary_queued should be a bool, got %T", sq)
	}

	// Verify consolidation_queued field exists
	cq, ok := decoded["consolidation_queued"]
	if !ok {
		t.Fatal("endSessionResponse JSON missing required field: consolidation_queued")
	}
	if _, isBool := cq.(bool); !isBool {
		t.Fatalf("consolidation_queued should be a bool, got %T", cq)
	}
}

// =============================================================================
// #46: Session/commit response — sha → commit_sha
// =============================================================================

// TestCommitResponse_UsesCommitSHA verifies that commitResponse JSON uses
// "commit_sha" field name instead of "sha".
//
// This test does NOT reference the Go field name (SHA or CommitSHA) — it only
// checks JSON output keys, so it compiles both before and after the struct change.
// =============================================================================
// #15: POST /v1/api/session/start — HandleStartSession
// =============================================================================

// TestHandleStartSession_NilService verifies that HandleStartSession returns
// 503 Service Unavailable when the session service is nil.
func TestHandleStartSession_NilService(t *testing.T) {
	h := &RESTHandler{sessionSvc: nil}

	body := `{"team_id":"team-123"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/api/session/start", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleStartSession(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("HandleStartSession with nil sessionSvc returned status %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	var decoded map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("response body is not valid JSON: %v", err)
	}

	if decoded["error"] != "session service not configured" {
		t.Errorf("error message = %q, want %q", decoded["error"], "session service not configured")
	}
	if decoded["code"] != "SERVICE_UNAVAILABLE" {
		t.Errorf("error code = %q, want %q", decoded["code"], "SERVICE_UNAVAILABLE")
	}
}

// TestHandleStartSession_NoAuth verifies that HandleStartSession returns
// 401 Unauthorized when no user_id is present in the request context.
func TestHandleStartSession_NoAuth(t *testing.T) {
	h := &RESTHandler{sessionSvc: &service.SessionService{}}

	body := `{"team_id":"team-123"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/api/session/start", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleStartSession(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("HandleStartSession with no auth returned status %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	var decoded map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("response body is not valid JSON: %v", err)
	}

	if decoded["error"] != "authentication required" {
		t.Errorf("error message = %q, want %q", decoded["error"], "authentication required")
	}
	if decoded["code"] != "UNAUTHORIZED" {
		t.Errorf("error code = %q, want %q", decoded["code"], "UNAUTHORIZED")
	}
}

// TestStartSessionResponse_HasFields verifies that startSessionResponse JSON
// includes session_id, started_at, and status fields.
func TestStartSessionResponse_HasFields(t *testing.T) {
	resp := startSessionResponse{
		SessionID: "sess-1",
		StartedAt: "2026-06-24T12:00:00Z",
		Status:    "active",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal startSessionResponse: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	// Verify session_id field exists
	if _, ok := decoded["session_id"]; !ok {
		t.Fatal("startSessionResponse JSON missing required field: session_id")
	}

	// Verify started_at field exists and is a string
	sa, ok := decoded["started_at"]
	if !ok {
		t.Fatal("startSessionResponse JSON missing required field: started_at")
	}
	if _, isStr := sa.(string); !isStr {
		t.Fatalf("started_at should be a string, got %T", sa)
	}

	// Verify status field exists and is "active"
	st, ok := decoded["status"]
	if !ok {
		t.Fatal("startSessionResponse JSON missing required field: status")
	}
	if st != "active" {
		t.Errorf("status = %q, want %q", st, "active")
	}
}

func TestCommitResponse_UsesCommitSHA(t *testing.T) {
	resp := commitResponse{
		SessionID: "sess-1",
		Status:    "linked",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal commitResponse: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	// Must have "commit_sha" field
	// Before fix: struct has SHA (json:"sha"), no CommitSHA.
	//   → "commit_sha" key does NOT exist in JSON → FAIL
	// After fix:  struct has CommitSHA (json:"commit_sha"), no SHA.
	//   → "commit_sha" key exists in JSON → PASS
	if _, ok := decoded["commit_sha"]; !ok {
		t.Fatal("commitResponse JSON must have 'commit_sha' field (not 'sha')")
	}

	// Must NOT have "sha" field (old name)
	// Before fix: "sha" key exists in JSON → FAIL
	// After fix:  "sha" key does NOT exist in JSON → PASS
	if _, ok := decoded["sha"]; ok {
		t.Fatal("commitResponse JSON must NOT have 'sha' field (renamed to 'commit_sha')")
	}
}
