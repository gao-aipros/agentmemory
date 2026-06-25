package service

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
)

var spaceCollapseRe = regexp.MustCompile(`\s+`)

// InsightFingerprint computes a content-based ID for deduplication.
// Normalization: lowercase, trim, collapse consecutive whitespace to single space.
// Returns "ins_" prefix + 32 hex chars (first 16 bytes of SHA256).
func InsightFingerprint(content string) string {
	norm := strings.ToLower(strings.TrimSpace(content))
	norm = spaceCollapseRe.ReplaceAllString(norm, " ")
	h := sha256.Sum256([]byte(norm))
	return fmt.Sprintf("ins_%x", h[:16])
}
