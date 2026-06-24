package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

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
