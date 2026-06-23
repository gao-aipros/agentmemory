package handler

import (
	"encoding/json"
	"io"
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

// =============================================================================
// Issue #95: Case-insensitive XSS sanitization — <SCRIPT> / <ScRiPt> must be
// stripped from query parameters alongside lowercase <script>.
// =============================================================================

func TestSanitizeInput_CaseInsensitiveScriptTagInQuery(t *testing.T) {
	handler := SanitizeInputMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if strings.Contains(name, "SCRIPT") || strings.Contains(name, "script") {
			t.Errorf("HTML tag still present after sanitization: %q", name)
		}
		if name != "alert(1)" {
			t.Errorf("expected sanitized value 'alert(1)', got %q", name)
		}
	}))

	req := httptest.NewRequest("GET", "/?name=<SCRIPT>alert(1)</SCRIPT>", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)
}

func TestSanitizeInput_MixedCaseXSSPatternsInQuery(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"javascript URI", "javascript:alert(1)", "alert(1)"},
		{"JavaScript URI", "JavaScript:alert(1)", "alert(1)"},
		{"JAVASCRIPT URI", "JAVASCRIPT:alert(1)", "alert(1)"},
		{"onerror attribute", `"onerror="`, `""`},
		{"onError attribute", `"onError="`, `""`},
		{"onload attribute", `"onload="`, `""`},
		{"onLoad attribute", `"onLoad="`, `""`},
		{"data URI", `"data:text/html"`, `""`},
		{"Data URI", `"Data:text/html"`, `""`},
		{"DATA URI", `"DATA:TEXT/HTML"`, `""`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := SanitizeInputMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				val := r.URL.Query().Get("q")
				if val != tt.expected {
					t.Errorf("expected %q, got %q", tt.expected, val)
				}
			}))
			req := httptest.NewRequest("GET", "/?q="+tt.input, nil)
			handler.ServeHTTP(httptest.NewRecorder(), req)
		})
	}
}

// =============================================================================
// Issue #94: JSON body sanitization — XSS payloads in JSON request bodies
// must be stripped.
// =============================================================================

func TestSanitizeJSONBody_SanitizesScriptTags(t *testing.T) {
	handler := SanitizeJSONBodyMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		var data map[string]interface{}
		if err := json.Unmarshal(body, &data); err != nil {
			t.Fatalf("failed to unmarshal body: %v", err)
		}
		text, ok := data["text"].(string)
		if !ok {
			t.Fatalf("expected string field 'text', got %T", data["text"])
		}
		if strings.Contains(text, "script") || strings.Contains(text, "SCRIPT") {
			t.Errorf("script tag still present after JSON body sanitization: %q", text)
		}
		if text != "alert(1)" {
			t.Errorf("expected sanitized text 'alert(1)', got %q", text)
		}
	}))

	body := `{"text": "<script>alert(1)</script>"}`
	req := httptest.NewRequest("POST", "/", io.NopCloser(strings.NewReader(body)))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(httptest.NewRecorder(), req)
}

func TestSanitizeJSONBody_SanitizesCaseInsensitiveScriptTag(t *testing.T) {
	handler := SanitizeJSONBodyMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		var data map[string]interface{}
		if err := json.Unmarshal(body, &data); err != nil {
			t.Fatalf("failed to unmarshal body: %v, raw: %s", err, string(body))
		}
		text, ok := data["text"].(string)
		if !ok {
			t.Fatalf("expected string field 'text', got %T", data["text"])
		}
		if strings.Contains(text, "SCRIPT") {
			t.Errorf("SCRIPT tag still present after JSON body sanitization: %q", text)
		}
		if text != "alert(1)" {
			t.Errorf("expected sanitized text 'alert(1)', got %q", text)
		}
	}))

	body := `{"text": "<SCRIPT>alert(1)</SCRIPT>"}`
	req := httptest.NewRequest("POST", "/", io.NopCloser(strings.NewReader(body)))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(httptest.NewRecorder(), req)
}

func TestSanitizeJSONBody_SanitizesXSSPatterns(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		check    string // substring that should NOT be present after sanitization
	}{
		{"javascript URI", `{"url":"javascript:alert(1)"}`, "javascript:"},
		{"JavaScript URI", `{"url":"JavaScript:alert(1)"}`, "JavaScript:"},
		{"JAVASCRIPT URI", `{"url":"JAVASCRIPT:alert(1)"}`, "JAVASCRIPT:"},
		{"onerror", `{"html":"<img src=x onerror=alert(1)>"}`, "onerror="},
		{"onError", `{"html":"<img src=x onError=alert(1)>"}`, "onError="},
		{"data URI", `{"html":"<a href=data:text/html>"}`, "data:text/html"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := SanitizeJSONBodyMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("failed to read body: %v", err)
				}
				if strings.Contains(string(body), tt.check) {
					t.Errorf("XSS pattern %q still present after sanitization: %s", tt.check, string(body))
				}
			}))
			req := httptest.NewRequest("POST", "/", io.NopCloser(strings.NewReader(tt.input)))
			req.Header.Set("Content-Type", "application/json")
			handler.ServeHTTP(httptest.NewRecorder(), req)
		})
	}
}

func TestSanitizeJSONBody_SkipsNonJSONBodies(t *testing.T) {
	originalBody := "this is not json"
	var captured io.ReadCloser

	handler := SanitizeJSONBodyMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Body
	}))

	req := httptest.NewRequest("POST", "/", io.NopCloser(strings.NewReader(originalBody)))
	req.Header.Set("Content-Type", "text/plain")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	body, err := io.ReadAll(captured)
	if err != nil {
		t.Fatalf("failed to read captured body: %v", err)
	}
	if string(body) != originalBody {
		t.Errorf("expected non-JSON body to pass through unchanged, got %q", string(body))
	}
}

func TestSanitizeJSONBody_SkipsGETRequests(t *testing.T) {
	originalBody := `{"text": "<script>alert(1)</script>"}`
	var captured io.ReadCloser

	handler := SanitizeJSONBodyMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Body
	}))

	req := httptest.NewRequest("GET", "/", io.NopCloser(strings.NewReader(originalBody)))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	body, err := io.ReadAll(captured)
	if err != nil {
		t.Fatalf("failed to read captured body: %v", err)
	}
	if string(body) != originalBody {
		t.Errorf("expected GET body to pass through unchanged, got %q", string(body))
	}
}
