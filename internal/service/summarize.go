package service

import (
	"strings"
)

// SummarizeObservation is a lightweight view of an observation used for summarization.
type SummarizeObservation struct {
	Title     string
	Narrative string
	Facts     string
	Concepts  []string
}

// EstimateTokens provides a rough token count estimate using a simple heuristic:
// ~1 token per 4 characters for English text. This is an approximation;
// real tokenizers are more accurate but this is sufficient for chunking decisions.
func EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	// Rough heuristic: 4 characters per token for English text
	tokens := len(text) / 4
	if tokens < 1 {
		tokens = 1
	}
	return tokens
}

// ChunkObservations splits a list of observations into chunks that fit within
// the given token budget. Each chunk is a slice of observations whose combined
// token estimate does not exceed the budget.
func ChunkObservations(observations []SummarizeObservation, tokenBudget int) [][]SummarizeObservation {
	if len(observations) == 0 {
		return nil
	}

	var chunks [][]SummarizeObservation
	var currentChunk []SummarizeObservation
	currentTokens := 0

	for _, obs := range observations {
		obsTokens := EstimateTokens(obs.Title) + EstimateTokens(obs.Narrative) +
			EstimateTokens(obs.Facts)

		// Start a new chunk if this observation would exceed the budget
		// and we already have items in the current chunk
		if currentTokens+obsTokens > tokenBudget && len(currentChunk) > 0 {
			chunks = append(chunks, currentChunk)
			currentChunk = nil
			currentTokens = 0
		}

		currentChunk = append(currentChunk, obs)
		currentTokens += obsTokens
	}

	// Append the last chunk if it has items
	if len(currentChunk) > 0 {
		chunks = append(chunks, currentChunk)
	}

	return chunks
}

// BuildSummarizePrompt assembles observations into an LLM prompt for session summarization.
func BuildSummarizePrompt(observations []SummarizeObservation) string {
	var sb strings.Builder

	sb.WriteString("Summarize the following agent session observations. ")
	sb.WriteString("Provide a concise overview of what happened, key decisions made, and important context. ")
	sb.WriteString("Include extracted concepts.\n\n")

	for i, obs := range observations {
		sb.WriteString("--- Observation ")
		sb.WriteString(formatInt(i + 1))
		sb.WriteString(" ---\n")
		sb.WriteString("Title: ")
		sb.WriteString(obs.Title)
		sb.WriteString("\n")
		sb.WriteString("Narrative: ")
		sb.WriteString(obs.Narrative)
		sb.WriteString("\n")

		if obs.Facts != "" {
			sb.WriteString("Facts: ")
			sb.WriteString(obs.Facts)
			sb.WriteString("\n")
		}

		if len(obs.Concepts) > 0 {
			sb.WriteString("Concepts: ")
			sb.WriteString(strings.Join(obs.Concepts, ", "))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// formatInt is a simple integer formatter using rune conversion.
// Avoids importing fmt for this one small use case.
func formatInt(n int) string {
	if n == 0 {
		return "0"
	}

	var digits []rune
	for n > 0 {
		digits = append([]rune{rune('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
