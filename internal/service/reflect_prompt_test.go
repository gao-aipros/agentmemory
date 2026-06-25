package service

import (
	"strings"
	"testing"
)

// TestBuildReflectPrompt_ContainsClusterConcepts verifies the prompt contains
// all concept names from the cluster, formatted as a "## Concept Cluster" line.
func TestBuildReflectPrompt_ContainsClusterConcepts(t *testing.T) {
	cluster := ReflectCluster{
		Concepts: []string{"patterns", "testing", "refactoring"},
		Facts: []FactRef{
			{Fact: "TDD improves code quality", Confidence: 0.9},
		},
		Lessons: []LessonRef{
			{Content: "Write tests before code", Confidence: 0.85},
		},
	}

	prompt := BuildReflectPrompt(cluster)

	// Concepts should appear in a ## Concept Cluster line
	if !strings.Contains(prompt, "## Concept Cluster") {
		t.Error("prompt should contain '## Concept Cluster' section heading")
	}
	if !strings.Contains(prompt, "patterns, testing, refactoring") {
		t.Error("prompt should contain comma-separated concept list")
	}
}

// TestBuildReflectPrompt_ContainsFactsWithConfidence verifies the prompt contains
// fact text and confidence values in the v0 bracket format.
func TestBuildReflectPrompt_ContainsFactsWithConfidence(t *testing.T) {
	cluster := ReflectCluster{
		Concepts: []string{"observability"},
		Facts: []FactRef{
			{Fact: "Structured logging aids debugging", Confidence: 0.85},
			{Fact: "Distributed tracing requires propagation", Confidence: 0.92},
		},
		Lessons: nil,
	}

	prompt := BuildReflectPrompt(cluster)

	// Should contain "## Known Facts" section heading
	if !strings.Contains(prompt, "## Known Facts") {
		t.Error("prompt should contain '## Known Facts' section heading")
	}

	if !strings.Contains(prompt, "Structured logging aids debugging") {
		t.Error("prompt should contain fact text")
	}
	if !strings.Contains(prompt, "Distributed tracing requires propagation") {
		t.Error("prompt should contain second fact text")
	}
	// v0 uses [confidence=X.X] bracket notation
	if !strings.Contains(prompt, "[confidence=0.85]") {
		t.Error("prompt should contain confidence label in bracket notation for first fact")
	}
	if !strings.Contains(prompt, "[confidence=0.92]") {
		t.Error("prompt should contain confidence label in bracket notation for second fact")
	}
}

// TestBuildReflectPrompt_ContainsLessonsWithConfidence verifies the prompt contains
// lesson content and confidence values in the v0 bracket format.
func TestBuildReflectPrompt_ContainsLessonsWithConfidence(t *testing.T) {
	cluster := ReflectCluster{
		Concepts: []string{"project-management"},
		Facts:    nil,
		Lessons: []LessonRef{
			{Content: "Standup meetings keep team aligned", Confidence: 0.78},
			{Content: "Sprint retrospectives improve process", Confidence: 0.88},
		},
	}

	prompt := BuildReflectPrompt(cluster)

	// Should contain "## Lessons Learned" section heading
	if !strings.Contains(prompt, "## Lessons Learned") {
		t.Error("prompt should contain '## Lessons Learned' section heading")
	}

	if !strings.Contains(prompt, "Standup meetings keep team aligned") {
		t.Error("prompt should contain lesson content")
	}
	if !strings.Contains(prompt, "Sprint retrospectives improve process") {
		t.Error("prompt should contain second lesson content")
	}
	// v0 uses [confidence=X.X] bracket notation
	if !strings.Contains(prompt, "[confidence=0.78]") {
		t.Error("prompt should contain confidence label in bracket notation for first lesson")
	}
	if !strings.Contains(prompt, "[confidence=0.88]") {
		t.Error("prompt should contain confidence label in bracket notation for second lesson")
	}
}

// TestBuildReflectPrompt_EmptyCluster verifies the prompt handles clusters
// with no items gracefully, matching v0's behavior.
func TestBuildReflectPrompt_EmptyCluster(t *testing.T) {
	cluster := ReflectCluster{
		Concepts: []string{},
		Facts:    nil,
		Lessons:  nil,
	}

	prompt := BuildReflectPrompt(cluster)

	// v0 always writes "## Concept Cluster:" even with empty concepts
	if !strings.Contains(prompt, "## Concept Cluster") {
		t.Error("prompt should contain '## Concept Cluster' section heading even with empty concepts")
	}

	// Should not produce a "## Known Facts" section when there are no facts
	if strings.Contains(prompt, "## Known Facts") {
		t.Error("prompt should not contain '## Known Facts' section when no facts provided")
	}

	// Should not produce a "## Lessons Learned" section when there are no lessons
	if strings.Contains(prompt, "## Lessons Learned") {
		t.Error("prompt should not contain '## Lessons Learned' section when no lessons provided")
	}
}

// TestBuildReflectPrompt_IncludesSystemPrompt verifies that BuildReflectPrompt
// prepends the REFLECT_SYSTEM content as a system header, so it is passed
// to the LLM in the single prompt string.
func TestBuildReflectPrompt_IncludesSystemPrompt(t *testing.T) {
	cluster := ReflectCluster{
		Concepts: []string{"patterns", "testing"},
	}
	prompt := BuildReflectPrompt(cluster)

	if !strings.Contains(prompt, "You are a higher-order reasoning engine") {
		t.Error("BuildReflectPrompt output should contain REFLECT_SYSTEM content")
	}
	if !strings.Contains(prompt, "<insights>") {
		t.Error("BuildReflectPrompt output should contain REFLECT_SYSTEM XML tags")
	}
	if !strings.Contains(prompt, "## Concept Cluster") {
		t.Error("BuildReflectPrompt output should still contain the cluster data after system prompt")
	}
}

// TestReflectSystem verifies the REFLECT_SYSTEM constant is non-empty and
// contains the expected XML output instructions.
func TestReflectSystem(t *testing.T) {
	if REFLECT_SYSTEM == "" {
		t.Error("REFLECT_SYSTEM constant should not be empty")
	}

	if !strings.Contains(REFLECT_SYSTEM, "<insights>") {
		t.Error("REFLECT_SYSTEM should contain <insights> XML tag")
	}

	if !strings.Contains(REFLECT_SYSTEM, "confidence=") {
		t.Error("REFLECT_SYSTEM should mention confidence attribute")
	}

	if !strings.Contains(REFLECT_SYSTEM, "title=") {
		t.Error("REFLECT_SYSTEM should mention title attribute")
	}

	if !strings.Contains(REFLECT_SYSTEM, "</insights>") {
		t.Error("REFLECT_SYSTEM should contain closing </insights> tag")
	}
}
