package service

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"
)

// RetryWithBackoff executes fn with exponential backoff, retrying up to maxRetries times.
// baseDelay is the initial delay between retries, doubling each attempt up to maxDelay.
// If maxRetries is 0 or negative, fn is executed exactly once.
// Returns the last error if all retries are exhausted.
func RetryWithBackoff(ctx context.Context, maxRetries int, baseDelay, maxDelay time.Duration, fn func() error) error {
	if maxRetries <= 0 {
		return fn()
	}

	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Check context before each attempt
		select {
		case <-ctx.Done():
			return fmt.Errorf("retry cancelled: %w", ctx.Err())
		default:
		}

		if attempt > 0 {
			// Calculate delay with exponential backoff
			delay := time.Duration(math.Pow(2, float64(attempt-1))) * baseDelay
			if delay > maxDelay {
				delay = maxDelay
			}

			slog.Warn("retrying after backoff",
				"attempt", attempt,
				"max_retries", maxRetries,
				"delay_ms", delay.Milliseconds(),
				"last_error", lastErr,
			)

			// Wait with context awareness
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return fmt.Errorf("retry cancelled during backoff: %w", ctx.Err())
			case <-timer.C:
				// continue to next attempt
			}
		}

		lastErr = fn()
		if lastErr == nil {
			if attempt > 0 {
				slog.Info("retry succeeded", "attempt", attempt)
			}
			return nil
		}

		slog.Debug("attempt failed", "attempt", attempt, "error", lastErr)
	}

	return fmt.Errorf("all %d retries exhausted, last error: %w", maxRetries, lastErr)
}
