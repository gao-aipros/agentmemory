package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
)

// =============================================================================
// Issue #25: extractToken helper — extracts auth token from ?token= query param
// or Authorization: Bearer header
// =============================================================================

func TestExtractTokenFromQueryParam(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/socket?token=my-session-token", nil)
	got := extractToken(req)
	if got != "my-session-token" {
		t.Errorf("extractToken from query param = %q, want %q", got, "my-session-token")
	}
}

func TestExtractTokenFromAuthorizationHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/socket", nil)
	req.Header.Set("Authorization", "Bearer my-session-token")
	got := extractToken(req)
	if got != "my-session-token" {
		t.Errorf("extractToken from bearer header = %q, want %q", got, "my-session-token")
	}
}

func TestExtractTokenQueryParamTakesPriority(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/socket?token=query-token", nil)
	req.Header.Set("Authorization", "Bearer header-token")
	got := extractToken(req)
	if got != "query-token" {
		t.Errorf("extractToken query param priority = %q, want %q", got, "query-token")
	}
}

func TestExtractTokenEmptyWhenNeitherPresent(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/socket", nil)
	got := extractToken(req)
	if got != "" {
		t.Errorf("extractToken with no token = %q, want empty string", got)
	}
}

func TestExtractTokenFromEmptyQueryParam(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/socket?token=", nil)
	got := extractToken(req)
	if got != "" {
		t.Errorf("extractToken with empty query param = %q, want empty string", got)
	}
}

func TestExtractTokenFromHeaderWithoutBearerPrefix(t *testing.T) {
	// AuthMiddleware already strips Bearer before calling extractToken,
	// but extractToken itself should handle both cases
	req := httptest.NewRequest(http.MethodGet, "/v1/socket", nil)
	req.Header.Set("Authorization", "raw-token-without-bearer")
	got := extractToken(req)
	if got != "raw-token-without-bearer" {
		t.Errorf("extractToken without bearer prefix = %q, want %q", got, "raw-token-without-bearer")
	}
}

// =============================================================================
// Issue #5: CSP header must allow inline styles, inline scripts, and WebSocket
// =============================================================================

func TestSecurityHeadersMiddlewareCSPAllowsInlineStylesAndScriptsAndWebSocket(t *testing.T) {
	handler := SecurityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no-op
	}))

	req := httptest.NewRequest(http.MethodGet, "/viewer/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")

	// Must still include default-src 'self'
	if !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("CSP should contain default-src 'self', got: %s", csp)
	}

	// Must allow inline styles
	if !strings.Contains(csp, "style-src") {
		t.Errorf("CSP should contain style-src directive, got: %s", csp)
	}
	if !strings.Contains(csp, "'unsafe-inline'") {
		t.Errorf("CSP should contain 'unsafe-inline', got: %s", csp)
	}

	// Must allow inline scripts
	if !strings.Contains(csp, "script-src") {
		t.Errorf("CSP should contain script-src directive, got: %s", csp)
	}

	// Must allow WebSocket connections
	if !strings.Contains(csp, "connect-src") {
		t.Errorf("CSP should contain connect-src directive, got: %s", csp)
	}
	if !strings.Contains(csp, "ws:") || !strings.Contains(csp, "wss:") {
		t.Errorf("CSP connect-src should allow ws: and wss:, got: %s", csp)
	}
}

func TestSecurityHeadersCSPExactMatch(t *testing.T) {
	handler := SecurityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no-op
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	expected := "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; connect-src 'self' ws: wss:"

	if csp != expected {
		t.Errorf("CSP header mismatch\n  got:  %s\n  want: %s", csp, expected)
	}
}

// =============================================================================
// Issue #115: API key expiry mid-session — checkAPIKeyExpiry rejects expired keys
// =============================================================================

func TestCheckAPIKeyExpiry_Expired(t *testing.T) {
	apiKey := &store.ApiKey{
		ExpiresAt: pgtype.Timestamptz{
			Time:  time.Now().Add(-1 * time.Hour),
			Valid: true,
		},
	}

	err := checkAPIKeyExpiry(apiKey)
	if err == nil {
		t.Error("Expected error for expired API key, got nil")
	}
}

func TestCheckAPIKeyExpiry_NotExpired(t *testing.T) {
	apiKey := &store.ApiKey{
		ExpiresAt: pgtype.Timestamptz{
			Time:  time.Now().Add(1 * time.Hour),
			Valid: true,
		},
	}

	err := checkAPIKeyExpiry(apiKey)
	if err != nil {
		t.Errorf("Expected no error for non-expired API key, got: %v", err)
	}
}

func TestCheckAPIKeyExpiry_NoExpirySet(t *testing.T) {
	apiKey := &store.ApiKey{
		ExpiresAt: pgtype.Timestamptz{Valid: false},
	}

	err := checkAPIKeyExpiry(apiKey)
	if err != nil {
		t.Errorf("Expected no error for API key without expiry, got: %v", err)
	}
}
