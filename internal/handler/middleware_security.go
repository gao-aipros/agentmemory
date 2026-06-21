package handler

import (
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// htmlTagRegex matches HTML tags for basic XSS prevention via input sanitization.
var htmlTagRegex = regexp.MustCompile(`<[^>]*>`)

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
func stripXSSPatterns(s string) string {
	// Remove javascript: protocol URIs
	s = strings.ReplaceAll(s, "javascript:", "")
	// Remove on* event handlers
	s = strings.ReplaceAll(s, "onerror=", "")
	s = strings.ReplaceAll(s, "onload=", "")
	s = strings.ReplaceAll(s, "onclick=", "")
	// Remove data: URIs used for XSS
	s = strings.ReplaceAll(s, "data:text/html", "")
	return s
}

// RateLimitMiddleware is a placeholder for future rate limiting.
// Currently passes through all requests (no-op).
//
// TODO: Integrate with proper rate limiter (e.g., golang.org/x/time/rate or redis-based)
//
// Configuration:
//
//	AGENTMEMORY_RATE_LIMIT — requests per second (default: 100)
//
// The env var is read but not enforced yet. When implemented, this middleware
// should return HTTP 429 Too Many Requests when the rate limit is exceeded.
func RateLimitMiddleware(next http.Handler) http.Handler {
	// Read the configured rate limit (for future use)
	rateLimit := 100 // default
	if val := os.Getenv("AGENTMEMORY_RATE_LIMIT"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			rateLimit = parsed
		} else {
			log.Printf("[WARN] RateLimitMiddleware: invalid AGENTMEMORY_RATE_LIMIT value %q, using default %d", val, rateLimit)
		}
	}

	_ = rateLimit // placeholder — not yet enforced

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO: Implement rate limiting using a token bucket or sliding window
		// When rate limit is exceeded, respond with:
		//   w.Header().Set("Retry-After", "1")
		//   http.Error(w, `{"error":"rate_limit_exceeded"}`, http.StatusTooManyRequests)
		//   return

		// For now, pass through all requests
		next.ServeHTTP(w, r)
	})
}

// SecurityHeadersMiddleware sets security-related HTTP headers on every response
// to protect against common web vulnerabilities (XSS, clickjacking, MIME sniffing, etc.).
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		w.Header().Set("X-Permitted-Cross-Domain-Policies", "none")

		next.ServeHTTP(w, r)
	})
}
