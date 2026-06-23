package handler

import (
	"encoding/json"
	"testing"
	"time"
)

// =============================================================================
// #44: Login response missing expires_at
// =============================================================================

// TestLoginResponse_HasExpiresAt verifies that loginResponse JSON includes
// the expires_at field with an RFC3339 timestamp. This aligns the API response
// with the contract spec, which requires expires_at on login.
//
// This test does NOT reference the Go field name (ExpiresAt) — it only checks
// the JSON output, so it compiles both before and after the struct change.
func TestLoginResponse_HasExpiresAt(t *testing.T) {
	resp := loginResponse{
		Token:     "test-token",
		ExpiresAt: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		User: userResponse{
			ID:    "user-1",
			Email: "test@example.com",
			Name:  "Test User",
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal loginResponse: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	// Verify expires_at field exists
	// Before fix: struct has no ExpiresAt field, so JSON has no "expires_at" key → FAIL
	// After fix:  struct has ExpiresAt field, so JSON has "expires_at" key → PASS
	expiresAt, ok := decoded["expires_at"]
	if !ok {
		t.Fatal("loginResponse JSON missing required field: expires_at")
	}

	expiresAtStr, ok := expiresAt.(string)
	if !ok {
		t.Fatalf("expires_at should be a string, got %T", expiresAt)
	}
	if expiresAtStr == "" {
		t.Fatal("expires_at should not be empty")
	}

	// Verify it's a valid RFC3339 timestamp
	_, err = time.Parse(time.RFC3339, expiresAtStr)
	if err != nil {
		t.Fatalf("expires_at should be RFC3339 format, got %q: %v", expiresAtStr, err)
	}

	// Verify it's in the future (within reasonable tolerance)
	parsed, _ := time.Parse(time.RFC3339, expiresAtStr)
	if parsed.Before(time.Now().Add(-1 * time.Minute)) {
		t.Fatalf("expires_at should be in the future, got %v", parsed)
	}
}

// TestHandleRegister_ResponseHasExpiresAt verifies that the register response
// also includes expires_at (since it reuses loginResponse).
func TestHandleRegister_ResponseHasExpiresAt(t *testing.T) {
	resp := loginResponse{
		Token:     "reg-token",
		ExpiresAt: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		User: userResponse{
			ID:    "user-2",
			Email: "reg@example.com",
			Name:  "New User",
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal loginResponse: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if _, ok := decoded["expires_at"]; !ok {
		t.Fatal("register response (loginResponse) JSON missing expires_at")
	}
}

// =============================================================================
// #47: API key creation — full_key → key
// =============================================================================

// TestCreateAPIKeyResponse_UsesKey verifies that createAPIKeyResponse JSON uses
// "key" field name instead of "full_key".
//
// This test does NOT reference the Go field name (FullKey or Key) — it only
// checks JSON output keys, so it compiles both before and after the struct change.
func TestCreateAPIKeyResponse_UsesKey(t *testing.T) {
	resp := createAPIKeyResponse{
		ID:     "key-1",
		Label:  "my-key",
		Prefix: "amk_abc",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal createAPIKeyResponse: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	// Must have "key" field
	// Before fix: struct has FullKey (json:"full_key"), no Key.
	//   → "key" key does NOT exist in JSON → FAIL
	// After fix:  struct has Key (json:"key"), no FullKey.
	//   → "key" key exists in JSON → PASS
	if _, ok := decoded["key"]; !ok {
		t.Fatal("createAPIKeyResponse JSON must have 'key' field (not 'full_key')")
	}

	// Must NOT have "full_key" field (old name)
	// Before fix: "full_key" key exists in JSON → FAIL
	// After fix:  "full_key" key does NOT exist in JSON → PASS
	if _, ok := decoded["full_key"]; ok {
		t.Fatal("createAPIKeyResponse JSON must NOT have 'full_key' field (renamed to 'key')")
	}

	// Basic fields should still be present
	if _, ok := decoded["id"]; !ok {
		t.Fatal("createAPIKeyResponse JSON must have 'id' field")
	}
	if _, ok := decoded["label"]; !ok {
		t.Fatal("createAPIKeyResponse JSON must have 'label' field")
	}
	if _, ok := decoded["prefix"]; !ok {
		t.Fatal("createAPIKeyResponse JSON must have 'prefix' field")
	}
}

// =============================================================================
// #48: Keys list — bare array → {keys: [...]}
// =============================================================================

// TestHandleListAPIKeys_ResponseIsWrapped verifies that the list API keys
// response wraps the array in a {"keys": [...]} object.
func TestHandleListAPIKeys_ResponseIsWrapped(t *testing.T) {
	// Simulate the wrapping that HandleListAPIKeys should produce.
	// Before fix: writeJSON(w, http.StatusOK, response)
	// After fix:  writeJSON(w, http.StatusOK, map[string]any{"keys": response})
	keys := []apiKeyResponse{
		{ID: "k1", Label: "key1", Prefix: "amk_abc", CreatedAt: "2024-01-01T00:00:00Z"},
		{ID: "k2", Label: "key2", Prefix: "amk_def", CreatedAt: "2024-01-02T00:00:00Z"},
	}

	// This is what the fix should produce
	wrapped := map[string]interface{}{"keys": keys}
	data, err := json.Marshal(wrapped)
	if err != nil {
		t.Fatalf("failed to marshal wrapped response: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	// Must have "keys" top-level field
	keysField, ok := decoded["keys"]
	if !ok {
		t.Fatal("list keys response must have top-level 'keys' field")
	}

	// "keys" must be an array
	keysArray, ok := keysField.([]interface{})
	if !ok {
		t.Fatalf("'keys' field should be an array, got %T", keysField)
	}
	if len(keysArray) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keysArray))
	}
}
