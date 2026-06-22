package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// =============================================================================
// Task #43: Error code field — unit tests for writeError and httpStatusToCode
// =============================================================================

func TestHTTPStatusToCode(t *testing.T) {
	tests := []struct {
		status int
		code   string
	}{
		{http.StatusBadRequest, "BAD_REQUEST"},
		{http.StatusUnauthorized, "UNAUTHORIZED"},
		{http.StatusForbidden, "FORBIDDEN"},
		{http.StatusNotFound, "NOT_FOUND"},
		{http.StatusConflict, "CONFLICT"},
		{http.StatusTooManyRequests, "RATE_LIMITED"},
		{http.StatusInternalServerError, "INTERNAL_ERROR"},
		{http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE"},
	}

	for _, tc := range tests {
		t.Run(tc.code, func(t *testing.T) {
			got := httpStatusToCode(tc.status)
			if got != tc.code {
				t.Errorf("httpStatusToCode(%d) = %q, want %q", tc.status, got, tc.code)
			}
		})
	}
}

func TestWriteErrorProducesCodeField(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		message string
		expCode string
	}{
		{"bad request", http.StatusBadRequest, "invalid input", "BAD_REQUEST"},
		{"unauthorized", http.StatusUnauthorized, "auth required", "UNAUTHORIZED"},
		{"forbidden", http.StatusForbidden, "access denied", "FORBIDDEN"},
		{"not found", http.StatusNotFound, "resource missing", "NOT_FOUND"},
		{"conflict", http.StatusConflict, "already exists", "CONFLICT"},
		{"rate limited", http.StatusTooManyRequests, "slow down", "RATE_LIMITED"},
		{"internal error", http.StatusInternalServerError, "something broke", "INTERNAL_ERROR"},
		{"service unavailable", http.StatusServiceUnavailable, "down", "SERVICE_UNAVAILABLE"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeError(rec, tc.status, tc.message)

			// Verify status code
			if rec.Code != tc.status {
				t.Errorf("writeError status = %d, want %d", rec.Code, tc.status)
			}

			// Verify Content-Type
			ct := rec.Header().Get("Content-Type")
			if ct != "application/json" {
				t.Errorf("writeError Content-Type = %q, want %q", ct, "application/json")
			}

			// Verify body is valid JSON with both error and code fields
			var decoded map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
				t.Fatalf("writeError body is not valid JSON: %v", err)
			}

			if decoded["error"] != tc.message {
				t.Errorf("writeError error = %q, want %q", decoded["error"], tc.message)
			}

			if decoded["code"] != tc.expCode {
				t.Errorf("writeError code = %q, want %q", decoded["code"], tc.expCode)
			}
		})
	}
}

func TestWriteErrorWithUnknownStatus(t *testing.T) {
	// Unknown status should fall back to INTERNAL_ERROR
	rec := httptest.NewRecorder()
	writeError(rec, 418, "teapot")

	var decoded map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("writeError body is not valid JSON: %v", err)
	}

	if decoded["error"] != "teapot" {
		t.Errorf("error = %q, want %q", decoded["error"], "teapot")
	}

	// Unknown status codes should default to INTERNAL_ERROR
	if decoded["code"] != "INTERNAL_ERROR" {
		t.Errorf("code for unknown status = %q, want INTERNAL_ERROR", decoded["code"])
	}
}

func TestErrorResponseStruct(t *testing.T) {
	// Verify the ErrorResponse struct serializes correctly
	resp := ErrorResponse{
		Error: "test error",
		Code:  "TEST_CODE",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal ErrorResponse: %v", err)
	}

	var decoded map[string]string
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded["error"] != "test error" {
		t.Errorf("error = %q, want %q", decoded["error"], "test error")
	}
	if decoded["code"] != "TEST_CODE" {
		t.Errorf("code = %q, want %q", decoded["code"], "TEST_CODE")
	}
}
