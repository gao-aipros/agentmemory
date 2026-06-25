package service

import (
	"fmt"
	"strings"
)

// REFLECT_SYSTEM is the system prompt for the LLM reflect stage. It instructs
// the LLM to synthesize cross-cutting insights from a cluster of related
// memories and output them as XML.
const REFLECT_SYSTEM = `You are a higher-order reasoning engine. Given a cluster of related concepts, facts, lessons, and action outcomes, synthesize cross-cutting insights that span multiple individual memories.

Output format (XML):
<insights>
  <insight confidence="0.0-1.0" title="Short descriptive title">
    The higher-order observation or principle. Should be actionable and non-obvious — something that only becomes visible when viewing multiple memories together.
  </insight>
</insights>

Rules:
- Identify patterns, principles, or strategies that span 2+ source items
- Confidence reflects how well-supported the insight is across sources
- Title should be a concise label (under 60 chars)
- Content should be the actual observation (1-3 sentences)
- Prefer actionable insights over abstract summaries
- Skip insights that merely restate a single source item
- Always emit confidence attribute before title attribute`

// ReflectCluster groups related concepts, facts, and lessons for reflection.
type ReflectCluster struct {
	Concepts []string
	Facts    []FactRef
	Lessons  []LessonRef
}

// FactRef is a reference to a fact with a confidence score.
type FactRef struct {
	Fact       string
	Confidence float64
}

// LessonRef is a reference to a lesson with a confidence score.
type LessonRef struct {
	Content    string
	Confidence float64
}

// BuildReflectPrompt constructs the user prompt for the LLM reflect stage
// from a cluster of related concepts, facts, and lessons. It prepends the
// REFLECT_SYSTEM as a system header so the LLM receives format instructions
// alongside the cluster data in a single prompt string.
func BuildReflectPrompt(cluster ReflectCluster) string {
	var sb strings.Builder

	sb.WriteString(REFLECT_SYSTEM)
	sb.WriteString("\n\n---\n\n")

	sections := []string{fmt.Sprintf("## Concept Cluster: %s", strings.Join(cluster.Concepts, ", "))}

	if len(cluster.Facts) > 0 {
		lines := []string{"\n## Known Facts"}
		for _, f := range cluster.Facts {
			lines = append(lines, fmt.Sprintf("- [confidence=%v] %s", f.Confidence, f.Fact))
		}
		sections = append(sections, lines...)
	}

	if len(cluster.Lessons) > 0 {
		lines := []string{"\n## Lessons Learned"}
		for _, l := range cluster.Lessons {
			lines = append(lines, fmt.Sprintf("- [confidence=%v] %s", l.Confidence, l.Content))
		}
		sections = append(sections, lines...)
	}

	sb.WriteString(fmt.Sprintf("Synthesize higher-order insights from this cluster of related memories:\n\n%s", strings.Join(sections, "\n")))
	return sb.String()
}
