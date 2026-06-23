package handler

import (
	"bytes"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/go-chi/cors"
	"golang.org/x/time/rate"
)

// htmlTagRegex matches HTML tags for basic XSS prevention via input sanitization.
// Using (?i) flag for case-insensitive matching (<SCRIPT> / <script> / <ScRiPt> all match).
var htmlTagRegex = regexp.MustCompile(`(?i)<[^>]*>`)

// xssPatternRegex matches common XSS attack patterns case-insensitively.
// Patterns: javascript: URIs, on* event handlers, data:text/html URIs.
var xssPatternRegex = regexp.MustCompile(`(?i)javascript:|onerror=|onload=|onclick=|data:text/html`)

// SanitizeInputMiddleware strips HTML tags from query parameters to provide
// basic XSS prevention. It logs any sanitized inputs at DEBUG level (via log.Printf).
// This middleware rewrites the request URL with sanitized query parameters before
// passing the request to the next handler.
func SanitizeInputMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		originalQuery := r.URL.RawQuery
		if originalQuery == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Parse query parameters
		values, err := url.ParseQuery(originalQuery)
		if err != nil {
			// If query is malformed, pass through unchanged
			next.ServeHTTP(w, r)
			return
		}

		sanitized := false
		for key, vals := range values {
			for i, v := range vals {
				// Strip HTML tags using regex
				cleaned := htmlTagRegex.ReplaceAllString(v, "")
				// Also strip common XSS patterns
				cleaned = stripXSSPatterns(cleaned)
				if cleaned != v {
					sanitized = true
					log.Printf("[DEBUG] SanitizeInput: stripped HTML tags from query param %q (original length: %d, cleaned length: %d, remote: %s)",
						key, len(v), len(cleaned), r.RemoteAddr)
					values[key][i] = cleaned
				}
			}
		}

		if sanitized {
			r.URL.RawQuery = values.Encode()
		}

		next.ServeHTTP(w, r)
	})
}

// stripXSSPatterns removes common XSS attack patterns from a string.
// This is a basic defense-in-depth measure; a WAF should be used for production.
// Uses case-insensitive matching via xssPatternRegex to catch variants like
// JAVASCRIPT:, OnError=, DATA:TEXT/HTML, etc.
func stripXSSPatterns(s string) string {
	return xssPatternRegex.ReplaceAllString(s, "")
}

// SanitizeJSONBodyMiddleware strips HTML tags and XSS patterns from JSON
// request bodies. It reads the entire body, applies sanitization to the raw
// text, and replaces the body with the sanitized version.
//
// Only POST, PUT, and PATCH requests with a JSON Content-Type are processed.
// Non-JSON content types and GET/HEAD/OPTIONS requests pass through unchanged.
func SanitizeJSONBodyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only sanitize request methods that may contain a body
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
			// continue
		default:
			next.ServeHTTP(w, r)
			return
		}

		// Only process JSON content types
		ct := r.Header.Get("Content-Type")
		if !strings.Contains(ct, "/json") && !strings.Contains(ct, "+json") {
			next.ServeHTTP(w, r)
			return
		}

		// Read the full body
		body, err := io.ReadAll(r.Body)
		if err != nil || len(body) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		r.Body.Close()

		// Apply sanitization to raw body text
		original := string(body)
		cleaned := htmlTagRegex.ReplaceAllString(original, "")
		cleaned = stripXSSPatterns(cleaned)

		if cleaned != original {
			r.Body = io.NopCloser(bytes.NewReader([]byte(cleaned)))
			r.ContentLength = int64(len(cleaned))
		} else {
			r.Body = io.NopCloser(bytes.NewReader(body))
		}

		next.ServeHTTP(w, r)
	})
}

// RateLimitMiddleware enforces per-IP token bucket rate limiting using
// golang.org/x/time/rate. Each unique client IP gets its own rate limiter
// with a token bucket configured via AGENTMEMORY_RATE_LIMIT (default 100 req/s)
// and a burst size of rate/10 (minimum 5).
//
// When the rate limit is exceeded, the middleware returns HTTP 429 Too Many
// Requests with a JSON error body and does not call the next handler.
func RateLimitMiddleware(next http.Handler) http.Handler {
	rateLimit := parseRateLimit()
	log.Printf("[INFO] rate limiting configured: %d req/s (burst %d)", rateLimit, burstSize(rateLimit))

	var (
		mu    sync.Mutex
		limiters = make(map[string]*rate.Limiter)
	)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r.RemoteAddr)

		mu.Lock()
		lim, ok := limiters[ip]
		if !ok {
			burst := burstSize(rateLimit)
			lim = rate.NewLimiter(rate.Limit(rateLimit), burst)
			limiters[ip] = lim
		}
		mu.Unlock()

		if !lim.Allow() {
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// parseRateLimit reads AGENTMEMORY_RATE_LIMIT from the environment, defaulting to 100.
func parseRateLimit() int {
	s := os.Getenv("AGENTMEMORY_RATE_LIMIT")
	if s == "" {
		return 100
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		log.Printf("[WARN] AGENTMEMORY_RATE_LIMIT=%q is invalid; using default 100", s)
		return 100
	}
	return n
}

// burstSize returns the token bucket burst size for a given rate (rate/10, minimum 5).
func burstSize(rateLimit int) int {
	b := rateLimit / 10
	if b < 5 {
		b = 5
	}
	return b
}

// extractIP strips the port from r.RemoteAddr, returning just the IP.
func extractIP(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

// CORSMiddleware returns a CORS handler configured with the provided allowed origins.
// It wraps github.com/go-chi/cors with standard production defaults for methods,
// headers, credentials, and preflight caching.
func CORSMiddleware(allowedOrigins []string) func(next http.Handler) http.Handler {
	corsHandler := cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           86400,
	})
	return corsHandler
}

// SecurityHeadersMiddleware sets security-related HTTP headers on every response
// to protect against common web vulnerabilities (XSS, clickjacking, MIME sniffing, etc.).
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; connect-src 'self' ws: wss:")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		w.Header().Set("X-Permitted-Cross-Domain-Policies", "none")

		next.ServeHTTP(w, r)
	})
}
