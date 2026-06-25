package service

import (
	"encoding/json"
	"regexp"
	"strings"
)

// camelCaseRE matches boundaries between lowercase and uppercase letters
// (e.g., "WebFetch" -> "Web_Fetch") to enable word-based matching.
var camelCaseRE = regexp.MustCompile(`([a-z])([A-Z])`)

// SyntheticResult holds the output of a zero-LLM heuristic compression.
// It matches the shape of CompressionResult so the two can be used interchangeably.
type SyntheticResult struct {
	CompressedText string   `json:"compressed_text"`
	Concepts       []string `json:"concepts"`
	Confidence     float64  `json:"confidence"`
}

// BuildSyntheticCompression creates a heuristic-based compressed summary
// from the given observation fields without calling an LLM.
// Used as a fallback when LLM compression fails or is unavailable.
// toolName and hookType are used for type inference; toolInputJSON is parsed
// for file path extraction only when files is nil; a non-nil files slice
// is used directly.
func BuildSyntheticCompression(title, narrative, toolName, hookType string, toolInputJSON string, files []string) SyntheticResult {
	obsType := inferType(toolName, hookType)

	// Use provided files directly when non-nil; otherwise fall back to parsing JSON.
	if files == nil {
		files = extractFiles(toolInputJSON)
	}

	// Build compressed text — start with observed content
	parts := make([]string, 0, 4)
	if obsType != "other" {
		parts = append(parts, "["+obsType+"]")
	}
	if title != "" {
		parts = append(parts, title+":")
	}
	if narrative != "" {
		parts = append(parts, narrative)
	}
	if len(parts) == 0 {
		parts = append(parts, "synthetic observation")
	}

	compressedText := truncate(strings.Join(parts, " "), 400)

	// Concepts come from extracted files
	concepts := make([]string, 0, len(files))
	for _, f := range files {
		concepts = append(concepts, f)
	}

	return SyntheticResult{
		CompressedText: compressedText,
		Concepts:       concepts,
		Confidence:     0.3,
	}
}

// inferType maps a tool name and hook type to an observation type string.
// Ported from agentmemory-v1's compress-synthetic.ts.
func inferType(toolName, hookType string) string {
	// Hook type overrides take precedence
	switch hookType {
	case "post_tool_failure":
		return "error"
	case "prompt_submit":
		return "conversation"
	case "subagent_stop", "task_completed":
		return "subagent"
	case "notification":
		return "notification"
	}

	if toolName == "" {
		return "other"
	}

	// Normalize camelCase and kebab-case into word chunks so we can match
	// substrings like "WebFetch" -> "web" / "fetch".
	n := toolName
	n = camelCaseRE.ReplaceAllString(n, "${1}_${2}")
	n = strings.ToLower(n)
	n = strings.ReplaceAll(n, "-", "_")
	n = strings.ReplaceAll(n, " ", "_")

	// Check each category
	if hasWord(n, "fetch") || hasWord(n, "http") || hasWord(n, "web") {
		return "web_fetch"
	}
	if hasWord(n, "grep") || hasWord(n, "search") || hasWord(n, "glob") || hasWord(n, "find") {
		return "search"
	}
	if hasWord(n, "bash") || hasWord(n, "shell") || hasWord(n, "exec") || hasWord(n, "run") {
		return "command_run"
	}
	if hasWord(n, "edit") || hasWord(n, "update") || hasWord(n, "patch") || hasWord(n, "replace") {
		return "file_edit"
	}
	if hasWord(n, "write") || hasWord(n, "create") {
		return "file_write"
	}
	if hasWord(n, "read") || hasWord(n, "view") {
		return "file_read"
	}
	if hasWord(n, "task") || hasWord(n, "agent") {
		return "subagent"
	}
	return "other"
}

// hasWord checks if word appears as a word within n. A "word" is delimited by
// underscores or appears at the start/end of the string. This matches the v1
// behavior which uses regex word boundary checking plus startsWith/endsWith.
func hasWord(n, word string) bool {
	if n == word {
		return true
	}
	if strings.HasPrefix(n, word) {
		return true
	}
	if strings.HasSuffix(n, word) {
		return true
	}
	// Middle boundary: underscore on both sides
	return strings.Contains(n, "_"+word+"_")
}

// extractFiles extracts file paths from a JSON tool input.
// Ported from agentmemory-v1's compress-synthetic.ts.
func extractFiles(toolInputJSON string) []string {
	if toolInputJSON == "" {
		return nil
	}
	var input map[string]interface{}
	if err := json.Unmarshal([]byte(toolInputJSON), &input); err != nil {
		return nil
	}

	keys := []string{"file_path", "filepath", "path", "filePath", "file", "pattern"}
	var files []string
	seen := make(map[string]struct{})
	for _, key := range keys {
		v, ok := input[key]
		if !ok {
			continue
		}
		s, ok := v.(string)
		if !ok {
			continue
		}
		if len(s) == 0 || len(s) >= 512 {
			continue
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		files = append(files, s)
	}
	return files
}

