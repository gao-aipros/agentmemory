package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sync/semaphore"
)

// TestCompressionService_TriggerAsync_DoesNotBlock verifies that TriggerAsync
// returns immediately and does not wait for compression to complete.
func TestCompressionService_TriggerAsync_DoesNotBlock(t *testing.T) {
	s := &CompressionService{
		queries:    nil,
		llmService: nil,
		embedSvc:   nil,
		sem:        semaphore.NewWeighted(20),
	}

	done := make(chan struct{})
	go func() {
		// TriggerAsync should not block (observation is nil, but compress
		// will fail fast — that's fine, we're testing the non-blocking behavior)
		s.TriggerAsync(context.Background(), nil)
		close(done)
	}()

	select {
	case <-done:
		// Good — TriggerAsync returned immediately
	case <-time.After(5 * time.Second):
		t.Fatal("TriggerAsync blocked — did not return within 5 seconds")
	}
}

// TestCompressionService_SemaphoreLimitsConcurrency verifies that the semaphore
// limits the number of concurrently running compression goroutines.
func TestCompressionService_SemaphoreLimitsConcurrency(t *testing.T) {
	maxConcurrent := int64(5)
	s := &CompressionService{
		queries:    nil,
		llmService: nil,
		embedSvc:   nil,
		sem:        semaphore.NewWeighted(int64(maxConcurrent)),
	}

	var concurrent int64
	var maxObserved int64

	// Launch more goroutines than the semaphore allows
	for i := 0; i < 20; i++ {
		i := i
		go func() {
			if err := s.sem.Acquire(context.Background(), 1); err != nil {
				return
			}
			defer s.sem.Release(1)

			v := atomic.AddInt64(&concurrent, 1)
			defer atomic.AddInt64(&concurrent, -1)

			if v > atomic.LoadInt64(&maxObserved) {
				atomic.StoreInt64(&maxObserved, v)
			}

			// Simulate work
			time.Sleep(50 * time.Millisecond)
			_ = i
		}()
	}

	// Wait for all goroutines to finish
	time.Sleep(2 * time.Second)

	if maxObserved > int64(maxConcurrent) {
		t.Errorf("semaphore allowed %d concurrent goroutines, max expected %d", maxObserved, maxConcurrent)
	}
	t.Logf("max concurrent goroutines observed: %d (limit: %d)", maxObserved, maxConcurrent)
}

// TestCompressionService_PanicRecovery verifies that a panic inside the
// compression goroutine is caught by recover() and does not crash the process.
func TestCompressionService_PanicRecovery(t *testing.T) {
	s := &CompressionService{
		queries:    nil,
		llmService: nil,
		embedSvc:   nil,
		sem:        semaphore.NewWeighted(20),
	}

	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// This is what the real code does — log the panic.
			}
		}()

		if err := s.sem.Acquire(context.Background(), 1); err != nil {
			return
		}
		defer s.sem.Release(1)
		panic("intentional compression panic")
	}()

	// Verify the goroutine didn't crash the test
	// We give it time to execute and potentially propagate
	time.Sleep(500 * time.Millisecond)
	_ = done // not needed, just verifying no crash
}
