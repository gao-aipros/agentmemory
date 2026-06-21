package service

import (
	"fmt"
	"strings"
	"time"
)

// ContextBudget defines the token budget for context injection.
// Total budget is 1500 tokens: ~1100 for source content, ~400 for formatting and recall IDs.
type ContextBudget struct {
	TotalTokens   int
	SourceTokens  int
	OverheadTokens int
}

// BucketAllocation holds the token allocation per source bucket.
type BucketAllocation struct {
	Graph         int
	Lessons       int
	Observations  int
	Recap         int
	WorkingMemory int
}

// DefaultContextBudget returns the standard 1500-token budget.
func DefaultContextBudget() ContextBudget {
	return ContextBudget{
		TotalTokens:   1500,
		SourceTokens:  1100,
		OverheadTokens: 400,
	}
}

// NewContextBudget creates a budget with the given total token limit.
// Allocations are computed proportionally to the default budget ratios.
func NewContextBudget(totalTokens int) ContextBudget {
	// Default ratio: source is 1100/1500 of total
	sourceTokens := int(float64(totalTokens) * (1100.0 / 1500.0))
	overheadTokens := totalTokens - sourceTokens
	return ContextBudget{
		TotalTokens:    totalTokens,
		SourceTokens:   sourceTokens,
		OverheadTokens: overheadTokens,
	}
}

// BucketAllocation returns the token allocation for each source bucket.
// Formula: total source tokens distributed across 5 buckets with weights:
// Graph: 20%, Lessons: 20%, Observations: 25%, Recap: 15%, WorkingMemory: 20%
func (b ContextBudget) BucketAllocation() BucketAllocation {
	src := b.SourceTokens
	return BucketAllocation{
		Graph:         int(float64(src) * 0.20),
		Lessons:       int(float64(src) * 0.20),
		Observations:  int(float64(src) * 0.25),
		Recap:         int(float64(src) * 0.15),
		WorkingMemory: int(float64(src) * 0.20),
	}
}

// contextSourceOrder defines the truncation priority order.
// Items appear in order from HIGHEST priority (trimmed LAST) to LOWEST priority (trimmed FIRST).
// Order: graph > lessons > observations > recap
var contextSourceOrder = []struct {
	name     string
	getText  func(*AssembledContext) string
	setText  func(*AssembledContext, string)
	priority int // 0 = highest (keep longest), 3 = lowest (trim first)
}{
	{"graph", func(a *AssembledContext) string { return a.Graph }, func(a *AssembledContext, s string) { a.Graph = s }, 0},
	{"lessons", func(a *AssembledContext) string { return a.Lessons }, func(a *AssembledContext, s string) { a.Lessons = s }, 1},
	{"observations", func(a *AssembledContext) string { return a.Observations }, func(a *AssembledContext, s string) { a.Observations = s }, 2},
	{"recap", func(a *AssembledContext) string { return a.Recap }, func(a *AssembledContext, s string) { a.Recap = s }, 3},
}

// ContextPriorityOrder returns the source type names in priority order
// (highest to lowest, meaning trimmed last to trimmed first).
// Order: graph → lessons → observations → recap
func ContextPriorityOrder() []string {
	return []string{"graph", "lessons", "observations", "recap"}
}

// ApplyBudget formats the assembled context, applies the token budget,
// and returns the formatted context string.
// Truncation order: recap → observations → lessons → graph
// Working memory is never truncated.
func ApplyBudget(assembled *AssembledContext, budget ContextBudget) string {
	if assembled == nil {
		return ""
	}

	// Step 1: Format each bucket with headers and recall IDs
	sections := []struct {
		header   string
		content  string
		priority int // higher = trimmed later
	}{
		{"### Memory Graph\n", assembled.Graph, 0},
		{"### Relevant Lessons\n", assembled.Lessons, 1},
		{"### Relevant Observations\n", assembled.Observations, 2},
		{"### Session Recap\n", assembled.Recap, 3},
	}

	// Step 2: Calculate total tokens
	var totalTokens int
	formatted := make([]string, len(sections))
	for i, sec := range sections {
		formatted[i] = sec.header + sec.content
		totalTokens += EstimateTokens(formatted[i])
	}

	// Add working memory if present
	workingMemoryFormatted := ""
	if assembled.WorkingMemory != "" {
		workingMemoryFormatted = "### Working Memory\n" + assembled.WorkingMemory
		totalTokens += EstimateTokens(workingMemoryFormatted)
	}

	// Step 3: Apply truncation if over budget
	// Truncate in priority order: lowest priority first (recap, then observations, then lessons, then graph)
	// Priority values: 3 = recap (trim first), 2 = observations, 1 = lessons, 0 = graph (trim last)
	if totalTokens > budget.TotalTokens {
		// Truncation order: recap (priority 3), observations (2), lessons (1), graph (0)
		for trimPriority := 3; trimPriority >= 0 && totalTokens > budget.TotalTokens; trimPriority-- {
			for i := range sections {
				if sections[i].priority == trimPriority {
					// Calculate how much we need to trim
					excess := totalTokens - budget.TotalTokens
					currentTokens := EstimateTokens(formatted[i])
					targetTokens := currentTokens - excess
					if targetTokens < EstimateTokens(sections[i].header) {
						targetTokens = EstimateTokens(sections[i].header)
					}

					// Truncate the content
					truncated := truncateToTokens(sections[i].content, targetTokens-EstimateTokens(sections[i].header))
					formatted[i] = sections[i].header + truncated

					// Recalculate total
					totalTokens = 0
					for _, f := range formatted {
						totalTokens += EstimateTokens(f)
					}
					if workingMemoryFormatted != "" {
						totalTokens += EstimateTokens(workingMemoryFormatted)
					}
				}
			}
		}
	}

	// Step 4: Build final output with date context
	now := time.Now().Format("2006-01-02")
	result := fmt.Sprintf("### Context (AgentMemory v2)\nDate: %s\n\n", now)

	for _, f := range formatted {
		if strings.TrimSpace(f) != strings.TrimSpace(fmt.Sprintf("%s", "")) {
			result += f + "\n\n"
		}
	}

	if workingMemoryFormatted != "" {
		result += workingMemoryFormatted + "\n"
	}

	// Ensure final result respects budget
	resultTokens := EstimateTokens(result)
	if resultTokens > budget.TotalTokens {
		// Hard truncation as last resort
		result = truncateToTokens(result, budget.TotalTokens)
	}

	return strings.TrimSpace(result)
}

// truncateToTokens truncates text to fit within a token budget.
// Uses the same 4 chars/token heuristic as EstimateTokens for consistency.
func truncateToTokens(text string, maxTokens int) string {
	if maxTokens <= 0 || text == "" {
		return ""
	}

	if maxTokens < 1 {
		maxTokens = 1
	}

	// 4 chars per token is the heuristic used by EstimateTokens
	maxChars := maxTokens * 4
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text
	}

	// Truncate at character boundary, trying to break at a space
	result := string(runes[:maxChars])
	lastSpace := strings.LastIndex(result, " ")
	if lastSpace > maxChars/2 {
		result = result[:lastSpace]
	}
	return result + "..."
}

// formatContextSection formats a single context section with header and content.
func formatContextSection(header, content string) string {
	if content == "" {
		return ""
	}
	return header + "\n" + content + "\n"
}
