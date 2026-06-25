package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseReflectResponse tests ParseReflectResponse which parses XML-formatted
// LLM reflect responses into structured ParsedInsight values.
func TestParseReflectResponse(t *testing.T) {
	t.Run("valid XML with multiple insights", func(t *testing.T) {
		input := `<insights>
<insight confidence="0.85" title="User prefers dark mode">The user consistently selects dark themes across all applications and has expressed a strong preference for reduced blue light exposure.</insight>
<insight confidence="0.72" title="Morning productivity peak">User completes the most coding tasks between 7 AM and 10 AM local time, with significantly fewer contributions after 2 PM.</insight>
</insights>`
		results := ParseReflectResponse(input)
		require.Len(t, results, 2)

		assert.InDelta(t, 0.85, results[0].Confidence, 1e-6)
		assert.Equal(t, "User prefers dark mode", results[0].Title)
		assert.Equal(t, "The user consistently selects dark themes across all applications and has expressed a strong preference for reduced blue light exposure.", results[0].Content)

		assert.InDelta(t, 0.72, results[1].Confidence, 1e-6)
		assert.Equal(t, "Morning productivity peak", results[1].Title)
		assert.Equal(t, "User completes the most coding tasks between 7 AM and 10 AM local time, with significantly fewer contributions after 2 PM.", results[1].Content)
	})

	t.Run("malformed XML missing closing tag on one insight", func(t *testing.T) {
		input := `<insights>
<insight confidence="0.90" title="Valid insight">This insight is complete and well-formed.</insight>
<insight confidence="0.60" title="Broken insight">This insight is missing its closing tag.
</insights>`
		results := ParseReflectResponse(input)
		require.Len(t, results, 1)
		assert.InDelta(t, 0.90, results[0].Confidence, 1e-6)
		assert.Equal(t, "Valid insight", results[0].Title)
		assert.Equal(t, "This insight is complete and well-formed.", results[0].Content)
	})

	t.Run("empty response", func(t *testing.T) {
		results := ParseReflectResponse("")
		assert.Empty(t, results)
		assert.Nil(t, results)
	})

	t.Run("missing confidence attribute", func(t *testing.T) {
		input := `<insights>
<insight title="No confidence here">This insight block has a title but no confidence attribute.</insight>
<insight confidence="0.50" title="Good insight">This one is properly formed.</insight>
</insights>`
		results := ParseReflectResponse(input)
		require.Len(t, results, 1)
		assert.InDelta(t, 0.50, results[0].Confidence, 1e-6)
		assert.Equal(t, "Good insight", results[0].Title)
	})

	t.Run("missing title attribute", func(t *testing.T) {
		input := `<insights>
<insight confidence="0.80">This insight block has confidence but no title attribute.</insight>
<insight confidence="0.50" title="Good insight">This one is properly formed.</insight>
</insights>`
		results := ParseReflectResponse(input)
		require.Len(t, results, 1)
		assert.InDelta(t, 0.50, results[0].Confidence, 1e-6)
		assert.Equal(t, "Good insight", results[0].Title)
	})

	t.Run("confidence out of range", func(t *testing.T) {
		input := `<insights>
<insight confidence="1.5" title="Too high">This confidence is above 1.0 and should be rejected.</insight>
<insight confidence="-0.3" title="Too low">This confidence is below 0.0 and should be rejected.</insight>
<insight confidence="0.65" title="Just right">This insight has a valid confidence value.</insight>
</insights>`
		results := ParseReflectResponse(input)
		require.Len(t, results, 1)
		assert.InDelta(t, 0.65, results[0].Confidence, 1e-6)
		assert.Equal(t, "Just right", results[0].Title)
	})

	t.Run("title exceeds 60 characters", func(t *testing.T) {
		input := `<insights>
<insight confidence="0.80" title="This title is way too long and exceeds the maximum allowed length of sixty characters for insight titles">This insight has an overly long title.</insight>
<insight confidence="0.55" title="Short title">This one has a reasonable title length.</insight>
</insights>`
		// Verify the long title actually exceeds 60 chars
		longTitle := "This title is way too long and exceeds the maximum allowed length of sixty characters for insight titles"
		assert.Greater(t, len(longTitle), 60)

		results := ParseReflectResponse(input)
		require.Len(t, results, 1)
		assert.InDelta(t, 0.55, results[0].Confidence, 1e-6)
		assert.Equal(t, "Short title", results[0].Title)
	})

	t.Run("content is only whitespace", func(t *testing.T) {
		input := `<insights>
<insight confidence="0.75" title="Whitespace only">   </insight>
<insight confidence="0.62" title="Real content">This insight has actual meaningful content to parse.</insight>
</insights>`
		results := ParseReflectResponse(input)
		require.Len(t, results, 1)
		assert.InDelta(t, 0.62, results[0].Confidence, 1e-6)
		assert.Equal(t, "Real content", results[0].Title)
	})

	t.Run("extra wrapper text around insights tags", func(t *testing.T) {
		input := `Here are the insights I've derived from the conversation:

<insights>
<insight confidence="0.92" title="Memory is working">The user values long-term memory persistence across sessions.</insight>
<insight confidence="0.88" title="Pattern recognition">The user frequently references past coding patterns when solving new problems.</insight>
</insights>

I hope this helps summarize the key findings.`
		results := ParseReflectResponse(input)
		require.Len(t, results, 2)
		assert.InDelta(t, 0.92, results[0].Confidence, 1e-6)
		assert.Equal(t, "Memory is working", results[0].Title)
		assert.InDelta(t, 0.88, results[1].Confidence, 1e-6)
		assert.Equal(t, "Pattern recognition", results[1].Title)
	})

	t.Run("attributes in reverse order (title before confidence)", func(t *testing.T) {
		input := `<insights>
<insight title="Reversed attributes" confidence="0.78">This insight declares title before confidence, opposite of typical order.</insight>
<insight title="Second insight" confidence="0.91">Another insight with reversed attribute order.</insight>
</insights>`
		results := ParseReflectResponse(input)
		require.Len(t, results, 2)
		assert.InDelta(t, 0.78, results[0].Confidence, 1e-6)
		assert.Equal(t, "Reversed attributes", results[0].Title)
		assert.Equal(t, "This insight declares title before confidence, opposite of typical order.", results[0].Content)

		assert.InDelta(t, 0.91, results[1].Confidence, 1e-6)
		assert.Equal(t, "Second insight", results[1].Title)
		assert.Equal(t, "Another insight with reversed attribute order.", results[1].Content)
	})
}
