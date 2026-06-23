package service

import (
	"context"
	"errors"
	"testing"
)

// mockEmbedder implements embeddings.Embedder with configurable failure.
type mockEmbedder struct {
	// failUntil is the number of calls to EmbedQuery that should fail before succeeding.
	failUntil int
	// callCount tracks the number of times EmbedQuery has been called.
	callCount int
	// err is the error returned for failing calls.
	err error
}

func (m *mockEmbedder) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	return nil, errors.New("not implemented")
}

func (m *mockEmbedder) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	m.callCount++
	if m.callCount <= m.failUntil {
		return nil, m.err
	}
	return []float32{0.1, 0.2, 0.3}, nil
}

func TestGenerateEmbedding_RetrySuccess(t *testing.T) {
	mock := &mockEmbedder{
		failUntil: 2,
		err:       errors.New("transient api error"),
	}
	svc := NewEmbeddingServiceWithEmbedder(mock)

	result, err := svc.GenerateEmbedding(context.Background(), "test text")
	if err != nil {
		t.Fatalf("expected success after retries, got error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected embedding of length 3, got %d", len(result))
	}
	if mock.callCount != 3 {
		t.Fatalf("expected 3 calls to EmbedQuery (2 failures + 1 success), got %d", mock.callCount)
	}
}

func TestGenerateEmbedding_RetryExhaustion(t *testing.T) {
	mock := &mockEmbedder{
		failUntil: 5, // always fail
		err:       errors.New("persistent api error"),
	}
	svc := NewEmbeddingServiceWithEmbedder(mock)

	_, err := svc.GenerateEmbedding(context.Background(), "test text")
	if err == nil {
		t.Fatal("expected error after exhausting retries, got nil")
	}
	// Should have attempted 3 retries + initial = 4 total calls (maxRetries=3, so loop runs attempt 0..3)
	if mock.callCount != 4 {
		t.Fatalf("expected 4 calls to EmbedQuery (3 retries exhausted), got %d", mock.callCount)
	}
}
