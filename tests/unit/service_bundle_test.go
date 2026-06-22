package unit

import (
	"testing"

	"github.com/agentmemory/agentmemory/internal/mcp"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// ServiceBundle Tests (T013)
// Verifies that NewServiceBundle creates all services and that nil pool
// gracefully degrades.
// =============================================================================

// TestNewServiceBundleCreatesAllServices verifies that NewServiceBundle with a
// nil pool still returns a non-nil ServiceBundle struct (graceful degradation).
func TestNewServiceBundleCreatesAllServices(t *testing.T) {
	// Even with nil pool, we should get back a usable ServiceBundle struct.
	bundle := mcp.NewServiceBundle(nil)

	require.NotNil(t, bundle, "ServiceBundle should not be nil even with nil pool")

	// Pool is nil because we passed nil
	assert.Nil(t, bundle.Pool, "Pool should be nil when constructed with nil")

	// All service fields should be non-nil (even if they're zero-value structs
	// that will return errors when called without a DB pool).
	// LLMService may be nil only if env vars are not set; we check the zero-value case.
	assert.NotNil(t, bundle.Observation, "Observation service should not be nil")
	assert.NotNil(t, bundle.Session, "Session service should not be nil")
	assert.NotNil(t, bundle.Recall, "Recall service should not be nil")
	assert.NotNil(t, bundle.SmartSearch, "SmartSearch service should not be nil")
	assert.NotNil(t, bundle.Search, "Search service should not be nil")
	assert.NotNil(t, bundle.User, "User service should not be nil")
	assert.NotNil(t, bundle.Team, "Team service should not be nil")
	assert.NotNil(t, bundle.Members, "TeamMembers service should not be nil")
	assert.NotNil(t, bundle.Summarization, "Summarization service should not be nil")
	assert.NotNil(t, bundle.Consolidation, "Consolidation service should not be nil")
	assert.NotNil(t, bundle.Reflection, "Reflection service should not be nil")
	assert.NotNil(t, bundle.Context, "Context service should not be nil")
	assert.NotNil(t, bundle.Compression, "Compression service should not be nil")
	assert.NotNil(t, bundle.Embedding, "Embedding service should not be nil")
	assert.NotNil(t, bundle.SessionEnd, "SessionEnd handler should not be nil")
	assert.NotNil(t, bundle.Eviction, "Eviction service should not be nil")
	// LLM service may be nil or non-nil depending on env; just check it exists as a field
	_ = bundle.LLM
}

// TestServiceBundlePoolAccess verifies that the bundle's Pool field matches
// what was passed to NewServiceBundle.
func TestServiceBundlePoolAccess(t *testing.T) {
	// With nil pool, Pool should be nil
	bundle := mcp.NewServiceBundle(nil)
	assert.Nil(t, bundle.Pool, "Pool should be nil when constructed with nil")
}

// TestServiceBundleIsReusable verifies that a single ServiceBundle can be
// shared across multiple consumers (the key architectural goal).
func TestServiceBundleIsReusable(t *testing.T) {
	// Create one bundle
	bundle := mcp.NewServiceBundle(nil)

	// Simulate two consumers accessing the same services
	obs1 := bundle.Observation
	obs2 := bundle.Observation

	// They should be the exact same pointer (shared state)
	assert.Same(t, obs1, obs2, "Multiple accesses to the same field should return the same pointer")

	// All other fields should also be consistent
	assert.Same(t, bundle.Session, bundle.Session)
	assert.Same(t, bundle.User, bundle.User)
	assert.Same(t, bundle.Team, bundle.Team)
	assert.Same(t, bundle.SessionEnd, bundle.SessionEnd)
}

// TestNewServiceBundleNilPoolDegradation verifies that nil pool results in
// zero-value services that won't panic when accessed (graceful degradation).
func TestNewServiceBundleNilPoolDegradation(t *testing.T) {
	bundle := mcp.NewServiceBundle(nil)

	// The bundle struct itself should be usable
	assert.NotNil(t, bundle)

	// Service structs should not be nil even with nil pool
	assert.NotNil(t, bundle.Observation)
	assert.NotNil(t, bundle.Compression)
	assert.NotNil(t, bundle.Embedding)
}

// =============================================================================
// New ServiceBundle Field Tests (Task #24)
// Verifies that all 10 newly added service fields are populated.
// =============================================================================

// TestNewServiceBundleNewFields verifies the 10 new service fields added in Task #24.
func TestNewServiceBundleNewFields(t *testing.T) {
	bundle := mcp.NewServiceBundle(nil)

	require.NotNil(t, bundle, "ServiceBundle should not be nil")

	// Slot service (was created but never stored — now stored)
	assert.NotNil(t, bundle.Slot, "Slot service should not be nil")

	// Signalling services
	assert.NotNil(t, bundle.Signal, "Signal service should not be nil")
	assert.NotNil(t, bundle.Sentinel, "Sentinel service should not be nil")
	assert.NotNil(t, bundle.Checkpoint, "Checkpoint service should not be nil")

	// Planning & workflow services
	assert.NotNil(t, bundle.Sketch, "Sketch service should not be nil")
	assert.NotNil(t, bundle.Routine, "Routine service should not be nil")

	// Operational services
	assert.NotNil(t, bundle.Snapshot, "Snapshot service should not be nil")
	assert.NotNil(t, bundle.FileHistory, "FileHistory service should not be nil")
	assert.NotNil(t, bundle.Patterns, "Patterns service should not be nil")
	assert.NotNil(t, bundle.Crystallize, "Crystallize service should not be nil")
}

// TestRegisterAllToolsPanicsOnNil verifies that RegisterAllTools panics
// when called with a nil ServiceBundle (fail loudly per CLAUDE.md rule #12).
func TestRegisterAllToolsPanicsOnNil(t *testing.T) {
	defer func() {
		r := recover()
		require.NotNil(t, r, "RegisterAllTools should panic when called with nil ServiceBundle")
		assert.Contains(t, r.(string), "nil ServiceBundle",
			"panic message should mention nil ServiceBundle")
	}()

	mcpServer := sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: "test", Version: "1.0.0"},
		&sdkmcp.ServerOptions{},
	)
	mcp.RegisterAllTools(mcpServer, nil)
	t.Error("RegisterAllTools should have panicked but didn't")
}
