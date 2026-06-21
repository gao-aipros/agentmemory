package service

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ConsolidationInput is the input for consolidation — typically a session summary
// plus metadata from the session it was derived from.
type ConsolidationInput struct {
	SessionID    string
	SummaryText  string
	Concepts     []string
	Observations int
}

// ConsolidationResult holds the parsed output of an LLM consolidation call.
type ConsolidationResult struct {
	Memories []ExtractedMemory `json:"memories"`
	Lessons  []ExtractedLesson `json:"lessons"`
}

// ExtractedMemory is a single semantic memory extracted from a session summary.
type ExtractedMemory struct {
	Content  string   `json:"content"`
	Concepts []string `json:"concepts"`
}

// ExtractedLesson is a single lesson learned extracted from a session summary.
type ExtractedLesson struct {
	Content string `json:"content"`
	Context string `json:"context"`
}

// BuildConsolidationPrompt assembles a session summary into an LLM prompt
// requesting extraction of semantic memories and lessons.
func BuildConsolidationPrompt(input *ConsolidationInput) string {
	var sb strings.Builder

	sb.WriteString("Extract key insights from the following agent session summary. ")
	sb.WriteString("Identify factual memories (things learned, decisions made) and ")
	sb.WriteString("actionable lessons (patterns to follow or avoid). ")
	sb.WriteString("Respond with JSON: {\"memories\": [{\"content\": \"...\", \"concepts\": [\"...\"]}], ")
	sb.WriteString("\"lessons\": [{\"content\": \"...\", \"context\": \"...\"}]}\n\n")

	sb.WriteString("Session Overview:\n")
	sb.WriteString(input.SummaryText)
	sb.WriteString("\n\n")

	if len(input.Concepts) > 0 {
		sb.WriteString("Key Concepts: ")
		sb.WriteString(strings.Join(input.Concepts, ", "))
		sb.WriteString("\n")
	}

	return sb.String()
}

// ParseConsolidationResponse parses the LLM JSON response into a ConsolidationResult.
func ParseConsolidationResponse(response string) (*ConsolidationResult, error) {
	var result ConsolidationResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return nil, fmt.Errorf("failed to parse consolidation response: %w", err)
	}
	return &result, nil
}
