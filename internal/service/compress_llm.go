package service

import (
	"encoding/json"
	"fmt"
	"strings"
)

// CompressionResult holds the output of an LLM compression call.
type CompressionResult struct {
	CompressedText string   `json:"compressed_text"`
	Concepts       []string `json:"concepts"`
}

// BuildCompressionPrompt assembles observation fields into an LLM prompt
// requesting a concise compressed summary with extracted concepts.
func BuildCompressionPrompt(title, narrative, facts string, concepts []string) string {
	var sb strings.Builder

	sb.WriteString("Compress the following observation into a concise summary. ")
	sb.WriteString("Extract key concepts as a list. ")
	sb.WriteString("Respond with JSON: {\"compressed_text\": \"...\", \"concepts\": [\"...\"]}\n\n")

	sb.WriteString("Title: ")
	sb.WriteString(title)
	sb.WriteString("\n")

	sb.WriteString("Narrative: ")
	sb.WriteString(narrative)
	sb.WriteString("\n")

	if facts != "" {
		sb.WriteString("Facts: ")
		sb.WriteString(facts)
		sb.WriteString("\n")
	}

	if len(concepts) > 0 {
		sb.WriteString("Concepts: ")
		sb.WriteString(strings.Join(concepts, ", "))
		sb.WriteString("\n")
	}

	return sb.String()
}

// ParseCompressionResponse parses the LLM JSON response into a CompressionResult.
// Returns an error if the JSON is invalid or required fields are missing.
func ParseCompressionResponse(response string) (string, []string, error) {
	var result CompressionResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return "", nil, fmt.Errorf("failed to parse compression response: %w", err)
	}

	if result.CompressedText == "" {
		return "", nil, fmt.Errorf("compressed_text is required but empty")
	}

	return result.CompressedText, result.Concepts, nil
}
