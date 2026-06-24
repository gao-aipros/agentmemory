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

// ObservationForCompression holds the fields needed for batch compression prompt building.
type ObservationForCompression struct {
	Title     string
	Narrative string
	Facts     string
	Concepts  []string
}

// BuildBatchCompressionPrompt assembles multiple observations into a single LLM prompt
// that requests batch compression. The response format is a JSON array of CompressionResult.
func BuildBatchCompressionPrompt(obs []ObservationForCompression) string {
	var sb strings.Builder

	sb.WriteString("Compress the following ")
	sb.WriteString(fmt.Sprintf("%d", len(obs)))
	sb.WriteString(" observations into concise summaries. ")
	sb.WriteString("Extract key concepts for each. ")
	sb.WriteString("Respond ONLY with a JSON array: [{\"compressed_text\": \"...\", \"concepts\": [\"...\"]}]\n\n")

	for i, o := range obs {
		idx := i + 1
		sb.WriteString(fmt.Sprintf("[%d] Title: %s | Narrative: %s", idx, o.Title, o.Narrative))
		if o.Facts != "" {
			sb.WriteString(fmt.Sprintf(" | Facts: %s", o.Facts))
		}
		if len(o.Concepts) > 0 {
			sb.WriteString(fmt.Sprintf(" | Concepts: %s", strings.Join(o.Concepts, ", ")))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// ParseBatchCompressionResponse parses the LLM JSON array response into a slice of
// CompressionResult. Validates that the count matches expectedCount.
func ParseBatchCompressionResponse(response string, expectedCount int) ([]CompressionResult, error) {
	var results []CompressionResult
	if err := json.Unmarshal([]byte(response), &results); err != nil {
		return nil, fmt.Errorf("failed to parse batch compression response: %w", err)
	}

	if len(results) != expectedCount {
		return nil, fmt.Errorf("expected %d compression results, got %d", expectedCount, len(results))
	}

	return results, nil
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
