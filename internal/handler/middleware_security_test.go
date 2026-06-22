package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// =============================================================================
// Issue #71: CORS middleware — handles preflight OPTIONS requests and sets
// appropriate Access-Control-* response headers.
// =============================================================================

func TestCORSPreflightHandling(t *testing.T) {
	// Create a chi router and mount CORS middleware.
	r := chi.NewRouter()
	r.Use(CORSMiddleware([]string{"*"}))

	// Add a catch-all route so the router can dispatch OPTIONS requests.
	r.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(r)
	defer ts.Close()

	// Send a CORS preflight request.
	req, err := http.NewRequest(http.MethodOptions, ts.URL+"/v1/api/observe", nil)
	if err != nil {
		t.Fatalf("failed to create OPTIONS request: %v", err)
	}
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to send OPTIONS request: %v", err)
	}
	defer resp.Body.Close()

	// Assert the response contains required CORS headers.
	allowOrigin := resp.Header.Get("Access-Control-Allow-Origin")
	if allowOrigin == "" {
		t.Error("expected Access-Control-Allow-Origin header to be present, but it was empty")
	}

	allowMethods := resp.Header.Get("Access-Control-Allow-Methods")
	if allowMethods == "" {
		t.Error("expected Access-Control-Allow-Methods header to be present, but it was empty")
	}

	maxAge := resp.Header.Get("Access-Control-Max-Age")
	if maxAge == "" {
		t.Error("expected Access-Control-Max-Age header to be present, but it was empty")
	}

	// When AllowedOrigins contains "*", go-chi/cors returns "*" literally.
	// This is valid per the spec; credentials are sent but the origin is not reflected.
	if allowOrigin != "*" && !containsCORSHeader(allowOrigin, "https://example.com") {
		t.Errorf("expected Access-Control-Allow-Origin to be '*' or include origin, got: %s", allowOrigin)
	}

	// Verify POST is among the allowed methods.
	if !strings.Contains(allowMethods, "POST") {
		t.Errorf("expected Access-Control-Allow-Methods to include POST, got: %s", allowMethods)
	}
}

func TestCORSSimpleRequestHeaders(t *testing.T) {
	r := chi.NewRouter()
	r.Use(CORSMiddleware([]string{"*"}))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	ts := httptest.NewServer(r)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/health", nil)
	if err != nil {
		t.Fatalf("failed to create GET request: %v", err)
	}
	req.Header.Set("Origin", "https://example.com")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to send GET request: %v", err)
	}
	defer resp.Body.Close()

	// Simple CORS requests should still have Allow-Origin.
	allowOrigin := resp.Header.Get("Access-Control-Allow-Origin")
	if allowOrigin == "" {
		t.Error("expected Access-Control-Allow-Origin header on simple GET request, but it was empty")
	}

	// go-chi/cors only sets Access-Control-Allow-Methods on preflight OPTIONS
	// responses, not on simple requests — that is spec-compliant behavior.
	// Verify credentials are allowed instead.
	allowCredentials := resp.Header.Get("Access-Control-Allow-Credentials")
	if allowCredentials != "true" {
		t.Errorf("expected Access-Control-Allow-Credentials to be 'true' on simple GET request, got: %s", allowCredentials)
	}
}

func TestCORSDisallowedOrigin(t *testing.T) {
	r := chi.NewRouter()
	r.Use(CORSMiddleware([]string{"https://example.com"}))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(r)
	defer ts.Close()

	// Send a preflight from a disallowed origin.
	req, err := http.NewRequest(http.MethodOptions, ts.URL+"/health", nil)
	if err != nil {
		t.Fatalf("failed to create OPTIONS request: %v", err)
	}
	req.Header.Set("Origin", "https://evil.com")
	req.Header.Set("Access-Control-Request-Method", "POST")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to send OPTIONS request: %v", err)
	}
	defer resp.Body.Close()

	// A disallowed origin should NOT receive a matching Allow-Origin.
	allowOrigin := resp.Header.Get("Access-Control-Allow-Origin")
	if allowOrigin == "https://evil.com" {
		t.Errorf("expected Access-Control-Allow-Origin NOT to be set for disallowed origin, got: %s", allowOrigin)
	}
}

// containsCORSHeader checks whether a comma-separated CORS header value
// contains the expected value, accounting for optional whitespace.
func containsCORSHeader(headerValue, expected string) bool {
	for _, part := range strings.Split(headerValue, ",") {
		if strings.TrimSpace(part) == expected {
			return true
		}
	}
	return false
}

// =============================================================================
// Issue #68: Rate limiting — RateLimitMiddleware is currently a no-op that
// passes through all requests. This test uses a real rate limiter to prove
// the middleware MUST be replaced with an enforcing implementation.
// =============================================================================

func TestRateLimitMiddlewareBlocksExcess(t *testing.T) {
	// Set a low rate limit so the test can trigger it with 10 rapid requests.
	t.Setenv("AGENTMEMORY_RATE_LIMIT", "2")

	r := chi.NewRouter()
	r.Use(RateLimitMiddleware)

	r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	ts := httptest.NewServer(r)
	defer ts.Close()

	// Send 10 rapid requests — far exceeding the 2 req/s with burst 1 limit.
	rateLimitedCount := 0
	okCount := 0

	for i := 0; i < 10; i++ {
		resp, err := http.Get(ts.URL + "/test")
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			rateLimitedCount++
		} else if resp.StatusCode == http.StatusOK {
			okCount++
		}
		resp.Body.Close()
	}

	// With rate=2 burst=1, after 2 requests the rest should be rate limited.
	// We expect at least some requests to be rate limited.
	if rateLimitedCount == 0 {
		t.Errorf(
			"expected some requests to receive HTTP 429 Too Many Requests, "+
				"but all 10 requests got HTTP 200 OK. "+
				"The RateLimitMiddleware is still a no-op.",
		)
	}

	t.Logf("rate limited: %d, ok: %d (total: %d)", rateLimitedCount, okCount, 10)
}
