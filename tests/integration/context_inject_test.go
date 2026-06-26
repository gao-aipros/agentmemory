package integration

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/agentmemory/agentmemory/internal/auth"
	"github.com/agentmemory/agentmemory/internal/mcp"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// T005 [P] [US1] Integration test: session_start context
// =============================================================================

// TestContextInjectSessionStart verifies that memory_inject_context with
// trigger=session_start returns non-empty context_text wrapped in
// <agentmemory-context> XML tags when the gate is enabled and data exists.
func TestContextInjectSessionStart(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	require.NoError(t, RunMigrations(db.Pool))
	require.NoError(t, SeedTestObservations(db.Pool))
	require.NoError(t, SeedTestLessons(db.Pool))
	require.NoError(t, SeedTestGraph(db.Pool))

	// Enable context injection gate
	os.Setenv("AGENTMEMORY_INJECT_CONTEXT", "true")
	defer os.Unsetenv("AGENTMEMORY_INJECT_CONTEXT")

	svc := mcp.NewServiceBundle(db.Pool)
	mcpServer := sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: "agentmemory-v2", Version: "2.0.0"},
		&sdkmcp.ServerOptions{},
	)

	// Auth middleware injects user ID into context so the tool handler
	// can authenticate — mirrors production auth middleware behavior.
	authMiddleware := func(next sdkmcp.MethodHandler) sdkmcp.MethodHandler {
		return func(ctx context.Context, method string, req sdkmcp.Request) (sdkmcp.Result, error) {
			ctx = context.WithValue(ctx, auth.UserIDKey, "user-001")
			return next(ctx, method, req)
		}
	}
	mcpServer.AddReceivingMiddleware(authMiddleware)
	mcp.RegisterAllTools(mcpServer, svc)

	inServer, inClient := sdkmcp.NewInMemoryTransports()
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	go mcpServer.Run(serverCtx, inServer)

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, inClient, nil)
	require.NoError(t, err)
	defer session.Close()

	// Call memory_inject_context with session_start trigger
	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "memory_inject_context",
		Arguments: map[string]interface{}{
			"trigger": "session_start",
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError, "tool should not return an error")

	// Parse response JSON
	require.Len(t, result.Content, 1)
	textContent, ok := result.Content[0].(*sdkmcp.TextContent)
	require.True(t, ok)

	var response map[string]interface{}
	err = json.Unmarshal([]byte(textContent.Text), &response)
	require.NoError(t, err)

	// Verify context_text is non-empty
	contextText, ok := response["context_text"].(string)
	require.True(t, ok, "response should have string context_text")
	assert.NotEmpty(t, contextText, "context_text should be non-empty when data exists")

	// Verify context_text is wrapped in <agentmemory-context> XML tags
	assert.Contains(t, contextText, "<agentmemory-context",
		"context_text should contain <agentmemory-context opening tag")
	assert.Contains(t, contextText, "</agentmemory-context>",
		"context_text should contain closing </agentmemory-context> tag")

	// Verify it's not skipped
	skipped, ok := response["skipped"].(bool)
	require.True(t, ok, "response should have bool skipped")
	assert.False(t, skipped, "should not be skipped when gate is enabled")

	// Verify hook_type is session_start
	hookType, ok := response["hook_type"].(string)
	require.True(t, ok, "response should have string hook_type")
	assert.Equal(t, "session_start", hookType)

	t.Logf("Context text length: %d", len(contextText))
	t.Logf("Context preview: %s", contextText[:min(len(contextText), 300)])
}

// =============================================================================
// T006 [P] [US1] Integration test: session_start with disabled gate
// =============================================================================

// TestContextInjectSessionStart_DisabledGate verifies that memory_inject_context
// with trigger=session_start returns skipped:true when the gate is disabled.
func TestContextInjectSessionStart_DisabledGate(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	require.NoError(t, RunMigrations(db.Pool))

	// Ensure gate is disabled (not set defaults to disabled)
	os.Unsetenv("AGENTMEMORY_INJECT_CONTEXT")

	svc := mcp.NewServiceBundle(db.Pool)
	mcpServer := sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: "agentmemory-v2", Version: "2.0.0"},
		&sdkmcp.ServerOptions{},
	)

	// Auth middleware injects user ID into context
	authMiddleware := func(next sdkmcp.MethodHandler) sdkmcp.MethodHandler {
		return func(ctx context.Context, method string, req sdkmcp.Request) (sdkmcp.Result, error) {
			ctx = context.WithValue(ctx, auth.UserIDKey, "user-001")
			return next(ctx, method, req)
		}
	}
	mcpServer.AddReceivingMiddleware(authMiddleware)
	mcp.RegisterAllTools(mcpServer, svc)

	inServer, inClient := sdkmcp.NewInMemoryTransports()
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	go mcpServer.Run(serverCtx, inServer)

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, inClient, nil)
	require.NoError(t, err)
	defer session.Close()

	// Call memory_inject_context with session_start trigger
	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "memory_inject_context",
		Arguments: map[string]interface{}{
			"trigger": "session_start",
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError, "tool should not return an error even when disabled")

	// Parse response JSON
	require.Len(t, result.Content, 1)
	textContent, ok := result.Content[0].(*sdkmcp.TextContent)
	require.True(t, ok)

	var response map[string]interface{}
	err = json.Unmarshal([]byte(textContent.Text), &response)
	require.NoError(t, err)

	// Verify skipped is true
	skipped, ok := response["skipped"].(bool)
	require.True(t, ok, "response should have bool skipped")
	assert.True(t, skipped, "should be skipped when gate is disabled")

	// Verify context_text is empty
	contextText, _ := response["context_text"].(string)
	assert.Empty(t, contextText, "context_text should be empty when disabled")

	// Verify hook_type is session_start
	hookType, ok := response["hook_type"].(string)
	require.True(t, ok, "response should have string hook_type")
	assert.Equal(t, "session_start", hookType)

	// Verify skip_reason explains why
	skipReason, ok := response["skip_reason"].(string)
	require.True(t, ok, "response should have string skip_reason")
	assert.Contains(t, skipReason, "disabled",
		"skip_reason should explain that context injection is disabled")
}
