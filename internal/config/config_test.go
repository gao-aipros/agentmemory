package config

import (
	"os"
	"testing"
	"time"
)

func TestConfigDefaultSchedulerIntervals(t *testing.T) {
	// Save env vars that might interfere
	saved := map[string]string{}
	envKeys := []string{
		"COMPRESSION_INTERVAL_MINUTES",
		"SUMMARIZATION_INTERVAL_MINUTES",
		"CONSOLIDATION_INTERVAL_MINUTES",
		"REFLECTION_INTERVAL_MINUTES",
	}
	for _, k := range envKeys {
		saved[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	defer func() {
		for k, v := range saved {
			if v != "" {
				os.Setenv(k, v)
			} else {
				os.Unsetenv(k)
			}
		}
	}()

	cfg := Load()

	// Verify default durations
	if cfg.CompressionInterval != 60*time.Minute {
		t.Errorf("CompressionInterval default: got %v, want %v", cfg.CompressionInterval, 60*time.Minute)
	}
	if cfg.SummarizationInterval != 120*time.Minute {
		t.Errorf("SummarizationInterval default: got %v, want %v", cfg.SummarizationInterval, 120*time.Minute)
	}
	if cfg.ConsolidationInterval != 360*time.Minute {
		t.Errorf("ConsolidationInterval default: got %v, want %v", cfg.ConsolidationInterval, 360*time.Minute)
	}
	if cfg.ReflectionInterval != 1440*time.Minute {
		t.Errorf("ReflectionInterval default: got %v, want %v", cfg.ReflectionInterval, 1440*time.Minute)
	}
}
