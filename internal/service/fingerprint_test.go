package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInsightFingerprint_SameContent_ProducesSameFingerprint(t *testing.T) {
	a := InsightFingerprint("The database connection pool is exhausted")
	b := InsightFingerprint("The database connection pool is exhausted")
	assert.Equal(t, a, b)
}

func TestInsightFingerprint_DifferentContent_ProducesDifferentFingerprints(t *testing.T) {
	a := InsightFingerprint("connection pool is exhausted")
	b := InsightFingerprint("memory usage is high")
	assert.NotEqual(t, a, b)
}

func TestInsightFingerprint_CaseInsensitive(t *testing.T) {
	a := InsightFingerprint("Connection Pool")
	b := InsightFingerprint("connection pool")
	assert.Equal(t, a, b)
}

func TestInsightFingerprint_WhitespaceInsensitive(t *testing.T) {
	a := InsightFingerprint("  hello   world  ")
	b := InsightFingerprint("hello world")
	assert.Equal(t, a, b)
}

func TestInsightFingerprint_PrefixPresent(t *testing.T) {
	fp := InsightFingerprint("any content here")
	assert.True(t, strings.HasPrefix(fp, "ins_"), "fingerprint should start with 'ins_'")
}

func TestInsightFingerprint_HexSuffixLength(t *testing.T) {
	fp := InsightFingerprint("any content here")
	parts := strings.SplitN(fp, "_", 2)
	assert.Len(t, parts, 2, "fingerprint should have exactly one 'ins_' prefix before the hex part")
	assert.Len(t, parts[1], 32, "hex suffix should be exactly 32 characters long")
}

func TestInsightFingerprint_EmptyContent(t *testing.T) {
	fp := InsightFingerprint("")
	assert.True(t, strings.HasPrefix(fp, "ins_"), "empty input should produce a valid fingerprint")
	assert.Len(t, fp, 36, "empty input fingerprint should be 36 characters total (ins_ + 32 hex)")
}
