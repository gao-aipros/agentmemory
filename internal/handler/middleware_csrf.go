package handler

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/agentmemory/agentmemory/internal/auth"
)

// csrfCookieName is the name of the CSRF token cookie.
const csrfCookieName = "csrf_token"

// csrfHeaderName is the name of the request header carrying the CSRF token.
const csrfHeaderName = "X-CSRF-Token"

// CSRFMiddleware protects state-changing endpoints (POST, PUT, DELETE, PATCH)
// from cross-site request forgery using the double-submit cookie pattern.
//
// Safe methods (GET, HEAD, OPTIONS) are not checked. On GET requests a random
// CSRF token cookie is set (Secure + HttpOnly + SameSite=Strict) for subsequent
// state-changing requests to use.
//
// API-key-authenticated requests (Bearer token with ak_ prefix) skip the CSRF
// check entirely — the API key header already provides proof of intent and
// cannot be read by a cross-origin attacker.
//
// State-changing requests must include an X-CSRF-Token header whose value
// matches the csrf_token cookie. If omitted or mismatched, a 403 Forbidden
// response is returned.
func CSRFMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Safe methods: set cookie on GET, pass through for all safe methods
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			if r.Method == http.MethodGet {
				setCSRFCookie(w)
			}
			next.ServeHTTP(w, r)
			return
		}

		// API-key-authenticated requests skip CSRF check — the Bearer token
		// header cannot be set by a cross-origin attacker and proves intent.
		token := extractToken(r)
		if strings.HasPrefix(token, auth.APIKeyPrefix) {
			next.ServeHTTP(w, r)
			return
		}

		// Double-submit cookie check: compare header value against cookie value
		cookie, err := r.Cookie(csrfCookieName)
		if err != nil {
			writeError(w, http.StatusForbidden, "CSRF token required")
			return
		}

		headerToken := r.Header.Get(csrfHeaderName)
		if headerToken == "" {
			writeError(w, http.StatusForbidden, "CSRF token required")
			return
		}

		if !constantTimeEqual(headerToken, cookie.Value) {
			writeError(w, http.StatusForbidden, "CSRF token mismatch")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// setCSRFCookie sets a random CSRF token cookie on the response.
// The cookie uses Secure + HttpOnly + SameSite=Strict for maximum security.
// If token generation fails, the cookie is silently omitted rather than
// breaking the request.
func setCSRFCookie(w http.ResponseWriter) {
	token, err := generateCSRFToken()
	if err != nil {
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

// generateCSRFToken generates a cryptographically random 32-byte token
// encoded as a hex string.
func generateCSRFToken() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// constantTimeEqual compares two strings in constant time to prevent
// timing side-channel attacks on the CSRF token comparison.
func constantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
