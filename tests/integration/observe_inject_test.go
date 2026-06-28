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
// T005 [P] [US1] Integration test: MCP memory_observe with inject:true,
// verify context_text in response
// =============================================================================

// TestObserveInject_WithInjectTrue verifies that memory_observe with
// inject:true and a context trigger type (session_start) returns both
// observation_id and context_text.
func TestObserveInject_WithInjectTrue(t *testing.T) {
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

	// Call memory_observe with inject:true and session_start trigger
	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "memory_observe",
		Arguments: map[string]interface{}{
			"type":       "session_start",
			"title":      "Test observe with inject",
			"narrative":  "Testing the inject parameter on memory_observe",
			"session_id": "sess-001",
			"inject":     true,
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

	// Verify observation is recorded
	obsID, ok := response["observation_id"].(string)
	require.True(t, ok, "response should have string observation_id")
	assert.NotEmpty(t, obsID, "observation_id should not be empty")

	// Verify session_id is present
	sid, ok := response["session_id"].(string)
	require.True(t, ok, "response should have string session_id")
	assert.Equal(t, "sess-001", sid)

	// Verify status is recorded
	status, ok := response["status"].(string)
	require.True(t, ok, "response should have string status")
	assert.Equal(t, "recorded", status)

	// Verify context_text is present
	contextText, ok := response["context_text"].(string)
	require.True(t, ok, "response should have string context_text when inject:true")
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

	// Verify skip_reason is empty or absent
	_, hasSkipReason := response["skip_reason"]
	assert.False(t, hasSkipReason, "skip_reason should not be present when not skipped")

	t.Logf("Context text length: %d", len(contextText))
	t.Logf("Context preview: %s", contextText[:min(len(contextText), 300)])
}

// =============================================================================
// T006 [P] [US1] Integration test: MCP memory_observe without inject,
// verify response unchanged (only observation_id, session_id, status)
// =============================================================================

// TestObserveInject_WithoutInject verifies that memory_observe without the
// inject parameter returns only the standard observation response fields
// and no context-related fields.
func TestObserveInject_WithoutInject(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	require.NoError(t, RunMigrations(db.Pool))
	require.NoError(t, SeedTestUser(db.Pool))
	require.NoError(t, SeedTestSession(db.Pool))

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

	// Call memory_observe WITHOUT inject parameter
	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "memory_observe",
		Arguments: map[string]interface{}{
			"type":       "session_start",
			"title":      "Test observe without inject",
			"narrative":  "Testing the default behavior without inject parameter",
			"session_id": "sess-001",
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

	// Verify standard fields exist
	obsID, ok := response["observation_id"].(string)
	require.True(t, ok, "response should have string observation_id")
	assert.NotEmpty(t, obsID, "observation_id should not be empty")

	sid, ok := response["session_id"].(string)
	require.True(t, ok, "response should have string session_id")
	assert.Equal(t, "sess-001", sid)

	status, ok := response["status"].(string)
	require.True(t, ok, "response should have string status")
	assert.Equal(t, "recorded", status)

	// Verify NO context-related fields (inject=false or absent)
	_, hasContextText := response["context_text"]
	assert.False(t, hasContextText,
		"context_text should NOT be present when inject is absent")

	_, hasSkipped := response["skipped"]
	assert.False(t, hasSkipped,
		"skipped should NOT be present when inject is absent")

	_, hasSkipReason := response["skip_reason"]
	assert.False(t, hasSkipReason,
		"skip_reason should NOT be present when inject is absent")
}

// =============================================================================
// T007 [P] [US1] Integration test: MCP memory_observe with inject:true
// and disabled gate, verify skipped=true
// =============================================================================

// TestObserveInject_DisabledGate verifies that memory_observe with inject:true
// records the observation but skips context injection when the gate is disabled.
func TestObserveInject_DisabledGate(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	require.NoError(t, RunMigrations(db.Pool))
	require.NoError(t, SeedTestUser(db.Pool))
	require.NoError(t, SeedTestSession(db.Pool))

	// Ensure gate is disabled (default when env var is not set)
	os.Unsetenv("AGENTMEMORY_INJECT_CONTEXT")

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

	// Call memory_observe with inject:true and a context trigger
	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "memory_observe",
		Arguments: map[string]interface{}{
			"type":       "session_start",
			"title":      "Test observe with disabled gate",
			"narrative":  "Testing the inject behavior when gate is disabled",
			"session_id": "sess-001",
			"inject":     true,
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError, "tool should not return an error even when gate is disabled")

	// Parse response JSON
	require.Len(t, result.Content, 1)
	textContent, ok := result.Content[0].(*sdkmcp.TextContent)
	require.True(t, ok)

	var response map[string]interface{}
	err = json.Unmarshal([]byte(textContent.Text), &response)
	require.NoError(t, err)

	// Verify observation is STILL recorded
	obsID, ok := response["observation_id"].(string)
	require.True(t, ok, "response should have string observation_id")
	assert.NotEmpty(t, obsID, "observation_id should not be empty (observation still recorded)")

	// Verify status
	status, ok := response["status"].(string)
	require.True(t, ok, "response should have string status")
	assert.Equal(t, "recorded", status)

	// Verify inject is skipped
	skipped, ok := response["skipped"].(bool)
	require.True(t, ok, "response should have bool skipped")
	assert.True(t, skipped, "inject should be skipped when gate is disabled")

	// Verify context_text is present but empty
	contextText, _ := response["context_text"].(string)
	assert.Empty(t, contextText, "context_text should be empty when gate is disabled")

	// Verify skip_reason explains why
	skipReason, ok := response["skip_reason"].(string)
	require.True(t, ok, "response should have string skip_reason")
	assert.Equal(t, "gate_disabled", skipReason,
		"skip_reason should be gate_disabled when context injection is disabled")
}

// =============================================================================
// T008 [P] [US1] Integration test: MCP memory_observe with inject:true but
// non-context trigger type, verify observation recorded, inject silently skipped
// =============================================================================

// TestObserveInject_NonContextTrigger verifies that memory_observe with
// inject:true and a non-context trigger type (e.g., post_tool_use) records
// the observation but silently skips context injection.
func TestObserveInject_NonContextTrigger(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	require.NoError(t, RunMigrations(db.Pool))
	require.NoError(t, SeedTestUser(db.Pool))
	require.NoError(t, SeedTestSession(db.Pool))

	// Enable the gate — even with the gate enabled,
	// non-context trigger types should not trigger injection
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

	// Call memory_observe with inject:true and a NON-context trigger type
	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "memory_observe",
		Arguments: map[string]interface{}{
			"type":       "post_tool_use",
			"title":      "Test observe with non-context trigger",
			"narrative":  "Testing inject behavior with post_tool_use trigger",
			"session_id": "sess-001",
			"inject":     true,
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

	// Verify observation IS recorded
	obsID, ok := response["observation_id"].(string)
	require.True(t, ok, "response should have string observation_id")
	assert.NotEmpty(t, obsID, "observation_id should not be empty (observation recorded)")

	// Verify status
	status, ok := response["status"].(string)
	require.True(t, ok, "response should have string status")
	assert.Equal(t, "recorded", status)

	// Verify session_id
	sid, ok := response["session_id"].(string)
	require.True(t, ok, "response should have string session_id")
	assert.Equal(t, "sess-001", sid)

	// Verify inject is skipped
	skipped, ok := response["skipped"].(bool)
	require.True(t, ok, "response should have bool skipped")
	assert.True(t, skipped, "inject should be skipped for non-context trigger type")

	// Verify context_text is present but empty
	contextText, _ := response["context_text"].(string)
	assert.Empty(t, contextText, "context_text should be empty for non-context trigger")

	// Verify skip_reason
	skipReason, ok := response["skip_reason"].(string)
	require.True(t, ok, "response should have string skip_reason")
	assert.Equal(t, "non_context_trigger_type", skipReason,
		"skip_reason should be non_context_trigger_type for non-context triggers")
}

// =============================================================================
// T009 [P] [US1] Integration test: MCP memory_observe with inject:true,
// pre_tool_use trigger, empty file_paths — verify inject skipped with reason
// =============================================================================

// TestObserveInject_PreToolUseEmptyFiles verifies that memory_observe with
// inject:true, type:pre_tool_use, and empty files array records the
// observation but skips context injection because no file paths were provided.
func TestObserveInject_PreToolUseEmptyFiles(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	require.NoError(t, RunMigrations(db.Pool))
	require.NoError(t, SeedTestUser(db.Pool))
	require.NoError(t, SeedTestSession(db.Pool))

	// Enable the gate
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

	// Call memory_observe with inject:true, type:pre_tool_use, and no files
	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "memory_observe",
		Arguments: map[string]interface{}{
			"type":       "pre_tool_use",
			"title":      "Test observe with pre_tool_use empty files",
			"narrative":  "Testing inject behavior with pre_tool_use and no file paths",
			"session_id": "sess-001",
			"inject":     true,
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

	// Verify observation IS recorded
	obsID, ok := response["observation_id"].(string)
	require.True(t, ok, "response should have string observation_id")
	assert.NotEmpty(t, obsID, "observation_id should not be empty (observation recorded)")

	// Verify status
	status, ok := response["status"].(string)
	require.True(t, ok, "response should have string status")
	assert.Equal(t, "recorded", status)

	// Verify inject is skipped
	skipped, ok := response["skipped"].(bool)
	require.True(t, ok, "response should have bool skipped")
	assert.True(t, skipped, "inject should be skipped when file_paths is empty for pre_tool_use")

	// Verify context_text is empty
	contextText, _ := response["context_text"].(string)
	assert.Empty(t, contextText, "context_text should be empty when inject is skipped")

	// Verify skip_reason explains why
	skipReason, ok := response["skip_reason"].(string)
	require.True(t, ok, "response should have string skip_reason")
	assert.Equal(t, "no_file_paths", skipReason,
		"skip_reason should be no_file_paths for empty file_paths")
}
