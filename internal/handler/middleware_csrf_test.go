package handler

import (
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// =============================================================================
// Issue #99: CSRF protection on state-changing endpoints
// =============================================================================

func TestCSRFGetSetsCookie(t *testing.T) {
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Check that Set-Cookie header contains csrf_token
	setCookie := rec.Header().Get("Set-Cookie")
	if !strings.Contains(setCookie, "csrf_token=") {
		t.Errorf("GET request should set csrf_token cookie, got Set-Cookie: %q", setCookie)
	}

	// Check cookie security attributes
	if !strings.Contains(setCookie, "HttpOnly") {
		t.Errorf("CSRF cookie should be HttpOnly, got: %q", setCookie)
	}
	if !strings.Contains(setCookie, "Secure") {
		t.Errorf("CSRF cookie should be Secure, got: %q", setCookie)
	}
	if !strings.Contains(setCookie, "SameSite=Strict") {
		t.Errorf("CSRF cookie should have SameSite=Strict, got: %q", setCookie)
	}
}

func TestCSRFPostWithoutTokenReturns403(t *testing.T) {
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("POST without CSRF token should return 403, got %d", rec.Code)
	}

	if !strings.Contains(rec.Body.String(), "CSRF") {
		t.Errorf("Response body should mention CSRF, got: %s", rec.Body.String())
	}
}

func TestCSRFPostWithValidTokenPasses(t *testing.T) {
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	token := hex.EncodeToString([]byte("this-is-a-32-byte-test-token!!!!"))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-CSRF-Token", token)
	req.AddCookie(&http.Cookie{
		Name:  "csrf_token",
		Value: token,
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("POST with valid CSRF token should pass, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("Expected body 'ok', got %q", rec.Body.String())
	}
}

func TestCSRFPostWithMismatchedTokenReturns403(t *testing.T) {
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-CSRF-Token", "cookie-token-value")
	req.AddCookie(&http.Cookie{
		Name:  "csrf_token",
		Value: "different-token-value",
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("POST with mismatched CSRF token should return 403, got %d", rec.Code)
	}
}

func TestCSRFAPIKeySkipsCSRF(t *testing.T) {
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer ak_testapikey12345678901234567890")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("API-key-authenticated POST should skip CSRF check, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("Expected body 'ok', got %q", rec.Body.String())
	}
}

func TestCSRFGetDoesNotRequireToken(t *testing.T) {
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET should not require CSRF token, got %d", rec.Code)
	}
}

func TestCSRFHeadSkipsCheck(t *testing.T) {
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodHead, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("HEAD should skip CSRF check, got %d", rec.Code)
	}
}

func TestCSRFOptionsSkipsCheck(t *testing.T) {
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("OPTIONS should skip CSRF check, got %d", rec.Code)
	}
}

func TestCSRFPutWithValidTokenPasses(t *testing.T) {
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	token := hex.EncodeToString([]byte("this-is-a-32-byte-test-token!!!!"))

	req := httptest.NewRequest(http.MethodPut, "/", nil)
	req.Header.Set("X-CSRF-Token", token)
	req.AddCookie(&http.Cookie{
		Name:  "csrf_token",
		Value: token,
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("PUT with valid CSRF token should pass, got %d", rec.Code)
	}
}

func TestCSRFDeleteWithValidTokenPasses(t *testing.T) {
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	token := hex.EncodeToString([]byte("this-is-a-32-byte-test-token!!!!"))

	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	req.Header.Set("X-CSRF-Token", token)
	req.AddCookie(&http.Cookie{
		Name:  "csrf_token",
		Value: token,
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("DELETE with valid CSRF token should pass, got %d", rec.Code)
	}
}
