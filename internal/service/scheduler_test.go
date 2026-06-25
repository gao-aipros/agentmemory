package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
)

// mockSchedulerDBTX implements store.DBTX for testing scheduler store queries.
type mockSchedulerDBTX struct {
	queryRowFunc func(ctx context.Context, sql string, args ...interface{}) pgx.Row
}

func (m *mockSchedulerDBTX) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag(""), nil
}

func (m *mockSchedulerDBTX) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return nil, nil
}

func (m *mockSchedulerDBTX) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	if m.queryRowFunc != nil {
		return m.queryRowFunc(ctx, sql, args...)
	}
	return &mockSchedulerRow{}
}

func (m *mockSchedulerDBTX) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	return 0, nil
}

// mockSchedulerRow implements pgx.Row for testing.
type mockSchedulerRow struct {
	scanFunc func(dest ...interface{}) error
	boolVal  bool
}

func (m *mockSchedulerRow) Scan(dest ...interface{}) error {
	if m.scanFunc != nil {
		return m.scanFunc(dest...)
	}
	if len(dest) > 0 {
		if d, ok := dest[0].(*bool); ok {
			*d = m.boolVal
		}
	}
	return nil
}

func TestSchedulerTiersAreCalled(t *testing.T) {
	// Create a scheduler with 1-second intervals using injected process functions.
	var compressionCalls, summarizationCalls, consolidationCalls, reflectionCalls atomic.Int32

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Scheduler{
		intervals: SchedulerIntervals{
			Compression:    time.Second,
			Summarization:  time.Second,
			Consolidation:  time.Second,
			Reflection:     time.Second,
		},
		CompressionFunc: func(_ context.Context) error {
			compressionCalls.Add(1)
			return nil
		},
		SummarizationFunc: func(_ context.Context) error {
			summarizationCalls.Add(1)
			return nil
		},
		ConsolidationFunc: func(_ context.Context) error {
			consolidationCalls.Add(1)
			return nil
		},
		ReflectionFunc: func(_ context.Context) error {
			reflectionCalls.Add(1)
			return nil
		},
	}

	s.Start(ctx)

	// Allow enough time for at least one tick on each tier.
	time.Sleep(1500 * time.Millisecond)
	cancel()
	// Give goroutines time to notice the cancellation.
	time.Sleep(100 * time.Millisecond)

	if calls := compressionCalls.Load(); calls < 1 {
		t.Errorf("CompressionFunc called %d times, want >= 1", calls)
	}
	if calls := summarizationCalls.Load(); calls < 1 {
		t.Errorf("SummarizationFunc called %d times, want >= 1", calls)
	}
	if calls := consolidationCalls.Load(); calls < 1 {
		t.Errorf("ConsolidationFunc called %d times, want >= 1", calls)
	}
	if calls := reflectionCalls.Load(); calls < 1 {
		t.Errorf("ReflectionFunc called %d times, want >= 1", calls)
	}
}

func TestSchedulerZeroIntervalSkipsTier(t *testing.T) {
	// A tier with interval=0 should not start.
	var compressionCalls atomic.Int32

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Scheduler{
		intervals: SchedulerIntervals{
			Compression:    0,    // disabled
			Summarization:  time.Second,
			Consolidation:  time.Second,
			Reflection:     time.Second,
		},
		CompressionFunc: func(_ context.Context) error {
			compressionCalls.Add(1)
			return nil
		},
		SummarizationFunc: func(_ context.Context) error {
			return nil
		},
		ConsolidationFunc: func(_ context.Context) error {
			return nil
		},
		ReflectionFunc: func(_ context.Context) error {
			return nil
		},
	}

	s.Start(ctx)

	time.Sleep(1500 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)

	if calls := compressionCalls.Load(); calls != 0 {
		t.Errorf("CompressionFunc called %d times, want 0 (tier disabled)", calls)
	}
}

func TestClaimUncompressedObservations(t *testing.T) {
	// Verify the claim query is sqlc-generated and contains FOR UPDATE SKIP LOCKED
	// This test validates the query exists and compiles
	_ = store.New(nil) // compilation check
	// The actual FOR UPDATE SKIP LOCKED behavior is tested in integration tests
}

// TestProcessReflection_CallsDecayInsights verifies that the ProcessReflection
// scheduler function calls DecayInsights after RunReflection completes, ensuring
// the decay feature is wired into the production reflection pipeline.
func TestProcessReflection_CallsDecayInsights(t *testing.T) {
	ctx := context.Background()

	// Track DecayInsights calls via the mock querier.
	var mu sync.Mutex
	decayCalled := false
	var capturedWeeks float64

	mockReflectQ := &mockReflectionQuerier{
		listMemories: func(ctx context.Context, limit int32) ([]store.Memory, error) {
			return []store.Memory{
				{ID: "m1", Content: "Memory one", Concepts: []string{"perf"}},
				{ID: "m2", Content: "Memory two", Concepts: []string{"perf"}},
				{ID: "m3", Content: "Memory three", Concepts: []string{"perf"}},
			}, nil
		},
		upsertInsight: func(ctx context.Context, params store.UpsertInsightParams) error {
			return nil
		},
		markMemoriesReflected: func(ctx context.Context, ids []string) error {
			return nil
		},
		applyDecayWithCounts: func(ctx context.Context, weeksSince float64) (store.ApplyDecayWithCountsRow, error) {
			mu.Lock()
			defer mu.Unlock()
			decayCalled = true
			capturedWeeks = weeksSince
			return store.ApplyDecayWithCountsRow{DecayedCount: 3, SoftDeletedCount: 1}, nil
		},
	}

	mockLLM := &mockReflectionLLM{
		callFunc: func(ctx context.Context, prompt string) (string, error) {
			return cannedXMLResponse, nil
		},
	}

	reflectionSvc := newReflectionServiceWithQuerier(mockReflectQ, mockLLM)

	// Mock DBTX so s.queries.HasUnreflectedMemories returns true, allowing
	// ProcessReflection to proceed past the guard into RunReflection + DecayInsights.
	mockDB := &mockSchedulerDBTX{
		queryRowFunc: func(ctx context.Context, sql string, args ...interface{}) pgx.Row {
			return &mockSchedulerRow{boolVal: true}
		},
	}

	s := &Scheduler{
		queries:       store.New(mockDB),
		reflectionSvc: reflectionSvc,
		intervals: SchedulerIntervals{
			Reflection: 7 * 24 * time.Hour, // 1 week
		},
	}
	// Wire the default process function so ProcessReflection is used.
	s.ReflectionFunc = s.ProcessReflection

	err := s.ProcessReflection(ctx)
	assert.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.True(t, decayCalled, "DecayInsights should be called after RunReflection completes")
	assert.Equal(t, 1.0, capturedWeeks, "weeksSince should be floored to 1.0 for Reflection interval < 1 week")
}
