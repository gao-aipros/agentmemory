package service

import (
	"os"
	"sync"
)

// ContextGate controls whether context injection is enabled.
// Reads the AGENTMEMORY_INJECT_CONTEXT environment variable once at startup.
// If false: skip context injection entirely (all hooks return empty).
// If true: proceed with injection per hook type rules.
type ContextGate struct {
	enabled bool
	mu      sync.RWMutex
}

// NewContextGate creates a new ContextGate, reading AGENTMEMORY_INJECT_CONTEXT
// from the environment (default: false).
func NewContextGate() *ContextGate {
	enabled := false
	if val := os.Getenv("AGENTMEMORY_INJECT_CONTEXT"); val != "" {
		enabled = val == "true" || val == "1" || val == "yes"
	}
	return &ContextGate{
		enabled: enabled,
	}
}

// NewContextGateWithValue creates a ContextGate with an explicit enabled value.
// Useful for testing.
func NewContextGateWithValue(enabled bool) *ContextGate {
	return &ContextGate{
		enabled: enabled,
	}
}

// IsEnabled returns true if context injection is enabled.
func (g *ContextGate) IsEnabled() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.enabled
}

// SetEnabled updates the context injection state at runtime.
func (g *ContextGate) SetEnabled(enabled bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.enabled = enabled
}
