package service

import (
	"context"
	"testing"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/stretchr/testify/assert"
)

// TestDecayInsights_ConfidenceDecrease verifies that DecayInsights calls
// ApplyDecay with the correct weeksSince parameter. The actual confidence
// computation (confidence - decay_rate * weeksSince, floored at GREATEST(0.05, ...))
// is handled by the SQL query — this test validates the service layer delegation.
func TestDecayInsights_ConfidenceDecrease(t *testing.T) {
	ctx := context.Background()

	var capturedWeeks float64
	mockQ := &mockReflectionQuerier{
		listMemories: func(ctx context.Context, limit int32) ([]store.Memory, error) {
			t.Error("ListAllMemories should not be called during DecayInsights")
			return nil, nil
		},
		upsertInsight: func(ctx context.Context, params store.UpsertInsightParams) error {
			t.Error("UpsertInsight should not be called during DecayInsights")
			return nil
		},
		markMemoriesReflected: func(ctx context.Context, ids []string) error {
			t.Error("MarkMemoriesReflected should not be called during DecayInsights")
			return nil
		},
		applyDecay: func(ctx context.Context, weeksSince float64) error {
			capturedWeeks = weeksSince
			return nil
		},
	}
	mockLLM := &mockReflectionLLM{}

	svc := newReflectionServiceWithQuerier(mockQ, mockLLM)
	decayed, softDeleted, err := svc.DecayInsights(ctx, 2.0)

	assert.NoError(t, err)
	assert.Equal(t, 2.0, capturedWeeks, "ApplyDecay should be called with weeksSince=2.0")
	// Counts are 0 from the thin wrapper; the SQL query handles all logic internally.
	assert.Equal(t, 0, decayed)
	assert.Equal(t, 0, softDeleted)
}

// TestDecayInsights_ReinforcedNotDecayed verifies that DecayInsights delegates
// to ApplyDecay. The SQL WHERE clause (last_reinforced_at IS NULL OR
// last_reinforced_at < now() - INTERVAL '1 week') ensures recently reinforced
// insights are not touched.
func TestDecayInsights_ReinforcedNotDecayed(t *testing.T) {
	ctx := context.Background()

	var capturedWeeks float64
	mockQ := &mockReflectionQuerier{
		applyDecay: func(ctx context.Context, weeksSince float64) error {
			capturedWeeks = weeksSince
			return nil
		},
	}
	mockLLM := &mockReflectionLLM{}

	svc := newReflectionServiceWithQuerier(mockQ, mockLLM)
	_, _, err := svc.DecayInsights(ctx, 1.0)

	assert.NoError(t, err)
	assert.Equal(t, 1.0, capturedWeeks,
		"DecayInsights should delegate to ApplyDecay which handles the reinforced-at condition in SQL")
}

// TestDecayInsights_SoftDelete verifies that DecayInsights delegates to ApplyDecay.
// The SQL CASE clause soft-deletes insights when confidence drops to ≤0.1 AND
// reinforcement_count = 0 (set deleted = true).
func TestDecayInsights_SoftDelete(t *testing.T) {
	ctx := context.Background()

	called := false
	mockQ := &mockReflectionQuerier{
		applyDecay: func(ctx context.Context, weeksSince float64) error {
			called = true
			return nil
		},
	}
	mockLLM := &mockReflectionLLM{}

	svc := newReflectionServiceWithQuerier(mockQ, mockLLM)
	_, _, err := svc.DecayInsights(ctx, 2.0)

	assert.NoError(t, err)
	assert.True(t, called, "ApplyDecay should be called; the SQL CASE handles soft-delete logic")
}

// TestDecayInsights_AlreadyDeleted verifies that DecayInsights delegates to ApplyDecay.
// The SQL WHERE clause (deleted = false) ensures already-deleted insights are skipped.
func TestDecayInsights_AlreadyDeleted(t *testing.T) {
	ctx := context.Background()

	called := false
	mockQ := &mockReflectionQuerier{
		applyDecay: func(ctx context.Context, weeksSince float64) error {
			called = true
			return nil
		},
	}
	mockLLM := &mockReflectionLLM{}

	svc := newReflectionServiceWithQuerier(mockQ, mockLLM)
	_, _, err := svc.DecayInsights(ctx, 3.0)

	assert.NoError(t, err)
	assert.True(t, called, "ApplyDecay should be called; the SQL WHERE clause handles the already-deleted guard")
}

// TestDecayInsights_ConfidenceFloor verifies that DecayInsights delegates to ApplyDecay.
// The SQL GREATEST(0.05, ...) ensures confidence never drops below 0.05.
func TestDecayInsights_ConfidenceFloor(t *testing.T) {
	ctx := context.Background()

	var capturedWeeks float64
	mockQ := &mockReflectionQuerier{
		applyDecay: func(ctx context.Context, weeksSince float64) error {
			capturedWeeks = weeksSince
			return nil
		},
	}
	mockLLM := &mockReflectionLLM{}

	svc := newReflectionServiceWithQuerier(mockQ, mockLLM)
	_, _, err := svc.DecayInsights(ctx, 3.0)

	assert.NoError(t, err)
	assert.Equal(t, 3.0, capturedWeeks,
		"DecayInsights should delegate to ApplyDecay which applies the GREATEST(0.05, ...) floor in SQL")
}

// TestDecayInsights_ErrorPropagation verifies that when ApplyDecay returns an
// error, DecayInsights propagates it as a wrapped error.
func TestDecayInsights_ErrorPropagation(t *testing.T) {
	ctx := context.Background()

	mockQ := &mockReflectionQuerier{
		applyDecay: func(ctx context.Context, weeksSince float64) error {
			return assert.AnError
		},
	}
	mockLLM := &mockReflectionLLM{}

	svc := newReflectionServiceWithQuerier(mockQ, mockLLM)
	decayed, softDeleted, err := svc.DecayInsights(ctx, 1.0)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to apply decay")
	assert.Equal(t, 0, decayed)
	assert.Equal(t, 0, softDeleted)
}
