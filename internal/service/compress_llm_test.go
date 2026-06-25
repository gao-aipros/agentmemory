package service

import (
	"strings"
	"testing"
)

// TestBuildBatchCompressionPrompt verifies that the batch prompt contains all
// observation titles, the required header, and the JSON array format instruction.
func TestBuildBatchCompressionPrompt(t *testing.T) {
	obs := []ObservationForCompression{
		{Title: "First observation", Narrative: "This is the first narrative content."},
		{Title: "Second observation", Narrative: "This is the second narrative content."},
		{Title: "Third observation", Narrative: "This is the third narrative content."},
	}

	prompt := BuildBatchCompressionPrompt(obs)

	// Assert prompt contains the header
	if !strings.Contains(prompt, "Compress the following") {
		t.Error("prompt should contain 'Compress the following' header")
	}

	// Assert prompt contains all observation titles
	if !strings.Contains(prompt, "First observation") {
		t.Error("prompt should contain title 'First observation'")
	}
	if !strings.Contains(prompt, "Second observation") {
		t.Error("prompt should contain title 'Second observation'")
	}
	if !strings.Contains(prompt, "Third observation") {
		t.Error("prompt should contain title 'Third observation'")
	}

	// Assert prompt mentions JSON array format
	if !strings.Contains(prompt, "JSON array") {
		t.Error("prompt should mention JSON array format")
	}

	// Assert prompt includes the observation count
	if !strings.Contains(prompt, "3") {
		t.Error("prompt should contain the observation count (3)")
	}
}

// TestParseBatchCompressionResponse tests parsing of the LLM response.
func TestParseBatchCompressionResponse(t *testing.T) {
	t.Run("valid JSON array with 3 items is parsed correctly", func(t *testing.T) {
		response := `[
			{"compressed_text": "Summary of first observation.", "concepts": ["concept1", "concept2"]},
			{"compressed_text": "Summary of second observation.", "concepts": ["concept3"]},
			{"compressed_text": "Summary of third observation.", "concepts": ["concept4", "concept5"]}
		]`
		results, err := ParseBatchCompressionResponse(response, 3)
		if err != nil {
			t.Fatalf("expected no error for valid JSON, got: %v", err)
		}
		if len(results) != 3 {
			t.Fatalf("expected 3 results, got %d", len(results))
		}
		if results[0].CompressedText != "Summary of first observation." {
			t.Errorf("expected first compressed_text, got %q", results[0].CompressedText)
		}
		if len(results[1].Concepts) != 1 || results[1].Concepts[0] != "concept3" {
			t.Errorf("expected second concepts to contain 'concept3', got %v", results[1].Concepts)
		}
	})

	t.Run("malformed JSON returns error", func(t *testing.T) {
		_, err := ParseBatchCompressionResponse("not valid json at all", 1)
		if err == nil {
			t.Fatal("expected error for malformed JSON, got nil")
		}
	})

	t.Run("wrong count (expected 3, got 2) returns error", func(t *testing.T) {
		response := `[
			{"compressed_text": "text1", "concepts": ["c1"]},
			{"compressed_text": "text2", "concepts": ["c2"]}
		]`
		_, err := ParseBatchCompressionResponse(response, 3)
			if err != nil {
				t.Fatal("expected no error for count mismatch, got:", err)
			}
	})

	t.Run("empty array with non-zero expected count returns error", func(t *testing.T) {
			results, err := ParseBatchCompressionResponse(`[]`, 1)
			if err != nil {
				t.Fatal("expected no error for empty array, got:", err)
			}
			if len(results) != 0 {
				t.Fatal("expected 0 results")
			}
	})
}
