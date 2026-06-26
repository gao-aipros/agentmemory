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

// =============================================================================
// T008 [P] [US2] Integration test: pre_tool_use with file_paths
// =============================================================================

// TestContextInjectPreToolUse_WithFilePaths verifies that memory_inject_context
// with trigger=pre_tool_use and file_paths returns non-empty context_text that
// references the requested file path.
func TestContextInjectPreToolUse_WithFilePaths(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	require.NoError(t, RunMigrations(db.Pool))
	require.NoError(t, SeedTestObservations(db.Pool))

	// Seed an observation about the specific file path we'll search for.
	// The TriggerPreToolUse handler uses the first file path as a hybrid search
	// query, so the narrative must match the search index.
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO observations (id, session_id, owner_type, owner_user_id, visibility, type, title, narrative, facts, concepts, files, importance, timestamp)
		VALUES ($1, 'sess-001', 'user', 'user-001', 'private', 'pre_tool_use', $2, $3, '', ARRAY['typescript', 'auth'], ARRAY[]::text[], 0.8, now())
		ON CONFLICT DO NOTHING
	`, "obs-pre-tool", "File Edit: some/file/path.ts",
		"Modified the authentication handler in some/file/path.ts to add JWT token validation logic.")
	require.NoError(t, err, "should seed file-specific observation")

	// Enable context injection gate
	os.Setenv("AGENTMEMORY_INJECT_CONTEXT", "true")
	defer os.Unsetenv("AGENTMEMORY_INJECT_CONTEXT")

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

	// Call memory_inject_context with pre_tool_use trigger and file_paths
	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "memory_inject_context",
		Arguments: map[string]interface{}{
			"trigger":    "pre_tool_use",
			"file_paths": []string{"some/file/path.ts"},
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
	assert.NotEmpty(t, contextText, "context_text should be non-empty when file-specific data exists")

	// Verify context_text is wrapped in <agentmemory-context> XML tags
	assert.Contains(t, contextText, "<agentmemory-context",
		"context_text should contain <agentmemory-context opening tag")
	assert.Contains(t, contextText, "</agentmemory-context>",
		"context_text should contain closing </agentmemory-context> tag")

	// Verify context_text references the file path
	assert.Contains(t, contextText, "some/file/path.ts",
		"context_text should reference the searched file path")

	// Verify it's not skipped
	skipped, ok := response["skipped"].(bool)
	require.True(t, ok, "response should have bool skipped")
	assert.False(t, skipped, "should not be skipped when gate is enabled with file paths")

	// Verify hook_type is pre_tool_use
	hookType, ok := response["hook_type"].(string)
	require.True(t, ok, "response should have string hook_type")
	assert.Equal(t, "pre_tool_use", hookType)

	t.Logf("Context text length: %d", len(contextText))
	t.Logf("Context preview: %s", contextText[:min(len(contextText), 300)])
}

// =============================================================================
// T009 [P] [US2] Integration test: pre_tool_use with empty file_paths
// =============================================================================

// TestContextInjectPreToolUse_EmptyFilePaths verifies that memory_inject_context
// with trigger=pre_tool_use and empty file_paths returns skipped:true with
// skip_reason "no file paths".
func TestContextInjectPreToolUse_EmptyFilePaths(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	require.NoError(t, RunMigrations(db.Pool))

	// Enable context injection gate
	os.Setenv("AGENTMEMORY_INJECT_CONTEXT", "true")
	defer os.Unsetenv("AGENTMEMORY_INJECT_CONTEXT")

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

	// Call memory_inject_context with pre_tool_use trigger and no file_paths (empty)
	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "memory_inject_context",
		Arguments: map[string]interface{}{
			"trigger":    "pre_tool_use",
			"file_paths": []string{},
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError, "tool should not return an error even with empty file paths")

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
	assert.True(t, skipped, "should be skipped when file_paths is empty")

	// Verify context_text is empty
	contextText, _ := response["context_text"].(string)
	assert.Empty(t, contextText, "context_text should be empty when file_paths is empty")

	// Verify hook_type is pre_tool_use
	hookType, ok := response["hook_type"].(string)
	require.True(t, ok, "response should have string hook_type")
	assert.Equal(t, "pre_tool_use", hookType)

	// Verify skip_reason explains why
	skipReason, ok := response["skip_reason"].(string)
	require.True(t, ok, "response should have string skip_reason")
	assert.Equal(t, "no file paths in tool input", skipReason,
		"skip_reason should indicate no file paths were provided")
}

// =============================================================================
// T011 [P] [US3] Integration test: pre_compact condensed context
// =============================================================================

// TestContextInjectPreCompact verifies that memory_inject_context with
// trigger=pre_compact returns condensed context (lessons + graph only,
// no observations or recap).
func TestContextInjectPreCompact(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	require.NoError(t, RunMigrations(db.Pool))
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

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "memory_inject_context",
		Arguments: map[string]interface{}{
			"trigger": "pre_compact",
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError, "tool should not return an error")

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
	assert.False(t, skipped, "should not be skipped when gate is enabled with data")

	// Verify hook_type is pre_compact
	hookType, ok := response["hook_type"].(string)
	require.True(t, ok, "response should have string hook_type")
	assert.Equal(t, "pre_compact", hookType)

	// PreCompact returns condensed context (lessons + graph + working_memory only).
	// Section headers appear but observation/recap buckets should be empty.
	// Verify the response structure is correct.
	assert.True(t, len(contextText) > 0, "context_text should be non-empty when data exists")

	t.Logf("Context text length: %d", len(contextText))
	t.Logf("Context preview: %s", contextText[:min(len(contextText), 300)])
}
