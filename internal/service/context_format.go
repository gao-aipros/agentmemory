package service

import (
	"fmt"
	"strings"
	"time"
)

// FormatObservationRef formats an observation reference line for context injection.
// Format: [obs_<id>] YYYY-MM-DD: <narrative excerpt>...
func FormatObservationRef(id, observationType, narrative string, timestamp time.Time) string {
	date := timestamp.Format("2006-01-02")
	// Truncate narrative to a reasonable excerpt length
	excerpt := truncate(narrative, 200)
	return fmt.Sprintf("[%s] %s %s: %s", id, date, observationType, excerpt)
}

// FormatLessonRef formats a lesson reference line for context injection.
// Format: [lesson_<id>] (confidence: 0.XX): <content excerpt>...
func FormatLessonRef(id string, confidence float64, content string) string {
	excerpt := truncate(content, 200)
	return fmt.Sprintf("[%s] (confidence: %.2f): %s", id, confidence, excerpt)
}

// FormatGraphRef formats a graph node reference line for context injection.
// Format: [graph_<id>] <label>: <narrative excerpt>...
func FormatGraphRef(id, label, narrative string) string {
	excerpt := truncate(narrative, 150)
	return fmt.Sprintf("[%s] %s: %s", id, label, excerpt)
}

// FormatRecapRef formats a session recap reference line.
// Format: [session_<id>] YYYY-MM-DD: <summary excerpt>...
func FormatRecapRef(sessionID string, summary string, timestamp time.Time) string {
	date := timestamp.Format("2006-01-02")
	shortID := sessionID
	if len(sessionID) > 12 {
		shortID = sessionID[:12]
	}
	excerpt := truncate(summary, 200)
	return fmt.Sprintf("[session_%s] %s: %s", shortID, date, excerpt)
}

// FormatWorkingMemoryRef formats a working memory slot reference.
// Format: [wm] <content excerpt>
func FormatWorkingMemoryRef(content string) string {
	excerpt := truncate(content, 300)
	return fmt.Sprintf("[wm] %s", excerpt)
}

// BuildContextHeader builds the top-level context header with metadata.
func BuildContextHeader() string {
	return fmt.Sprintf("### Context (AgentMemory v2)\nDate: %s\n\n", time.Now().Format("2006-01-02"))
}

// BuildContextFooter builds a footer with usage guidance.
func BuildContextFooter() string {
	return "\n\n---\nUse recall IDs (e.g., [obs_abc123]) to reference specific memories during this session."
}

// JoinContextRefs joins multiple formatted references with newlines.
func JoinContextRefs(refs []string) string {
	return strings.Join(refs, "\n")
}
