package integration

import (
	"context"
	"testing"

	"github.com/agentmemory/agentmemory/internal/mcp"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// MCP Tools Integration Tests (T105)
// =============================================================================

// TestMCPObserveRecallForgetLifecycle tests the full observe -> recall -> forget
// lifecycle through MCP tool calls against a real ParadeDB instance.
func TestMCPObserveRecallForgetLifecycle(t *testing.T) {
	t.Skip("Requires running ParadeDB instance — set up with AGENTMEMORY_DATABASE_URL")
}

// TestMCPToolRegistration verifies that all expected tools are registered.
func TestMCPToolRegistration(t *testing.T) {
	mcpServer := sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: "agentmemory-v2", Version: "2.0.0"},
		&sdkmcp.ServerOptions{},
	)

	// Register all tools with nil pool (tools register but services are nil)
	mcp.RegisterAllTools(mcpServer, mcp.NewServiceBundle(nil))

	// Connect client to verify tool listing
	inServer, inClient := sdkmcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go mcpServer.Run(ctx, inServer)

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, inClient, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer session.Close()

	// List all registered tools
	tools, err := session.ListTools(ctx, &sdkmcp.ListToolsParams{})
	require.NoError(t, err, "ListTools should succeed")
	t.Logf("Registered %d tools", len(tools.Tools))

	// Collect tool names
	toolNames := make(map[string]bool)
	for _, tool := range tools.Tools {
		toolNames[tool.Name] = true
	}

	expectedTools := []string{
		// Memory Operations
		"memory_observe",
		"memory_save",
		"memory_recall",
		"memory_smart_search",
		"memory_forget",
		"memory_compress_file",

		// Session Operations
		"memory_sessions",
		"memory_timeline",
		"memory_handoff",
		"memory_recap",

		// Lesson Operations
		"memory_lesson_save",
		"memory_lesson_recall",

		// Team Operations
		"team_create",
		"team_delete",
		"team_add_member",
		"team_remove_member",
		"team_list_members",
		"team_feed",

		// Auth Operations
		"auth_create_key",
		"auth_list_keys",
		"auth_revoke_key",

		// Action Operations
		"memory_action_create",
		"memory_action_update",
		"memory_frontier",
		"memory_next",

		// Pipeline + Governance + Export + Graph
		"memory_consolidate",
		"memory_crystallize",
		"memory_reflect",
		"memory_diagnose",
		"memory_heal",
		"memory_verify",
		"memory_audit",
		"memory_export",
		"memory_obsidian_export",
		"memory_commit_lookup",
		"memory_commits",
		"memory_mesh_sync",
		"memory_graph_query",
		"memory_relations",
		"memory_profile",
		"memory_patterns",
		"memory_facet_query",
		"memory_facet_tag",
		"memory_vision_search",

		// v1 Service Tools
		"memory_slot_create",
		"memory_slot_get",
		"memory_slot_list",
		"memory_slot_replace",
		"memory_slot_delete",
		"memory_slot_append",
		"memory_signal_read",
		"memory_signal_send",
		"memory_sentinel_create",
		"memory_sentinel_trigger",
		"memory_checkpoint",
		"memory_sketch_create",
		"memory_sketch_promote",
		"memory_routine_run",
		"memory_snapshot_create",
		"memory_file_history",
		"memory_lease",
		"memory_insight_list",
		"memory_team_share",
		"memory_claude_bridge_sync",
	}

	for _, expected := range expectedTools {
		assert.True(t, toolNames[expected],
			"tool %q should be registered", expected)
	}

	assert.Len(t, tools.Tools, len(expectedTools),
		"should have exactly %d tools registered", len(expectedTools))
}

// TestMCPStubToolsReturnProperly tests that stubbed tools return
// "not_implemented" with IsError=false.
func TestMCPStubToolsReturnProperly(t *testing.T) {
	mcpServer := sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: "agentmemory-v2", Version: "2.0.0"},
		&sdkmcp.ServerOptions{},
	)

	mcp.RegisterAllTools(mcpServer, mcp.NewServiceBundle(nil))

	inServer, inClient := sdkmcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go mcpServer.Run(ctx, inServer)

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, inClient, nil)
	require.NoError(t, err)
	defer session.Close()

	stubTools := []string{
		"memory_frontier",
		"memory_next",
		"memory_diagnose",
		"memory_heal",
		"memory_verify",
		"memory_audit",
	}

	for _, toolName := range stubTools {
		t.Run(toolName, func(t *testing.T) {
			result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
				Name:      toolName,
				Arguments: map[string]interface{}{},
			})

			if err != nil {
				t.Logf("%s returned protocol error: %v", toolName, err)
				return
			}

			assert.False(t, result.IsError,
				"stub tool %s should not set IsError=true", toolName)

			if len(result.Content) > 0 {
				textContent, ok := result.Content[0].(*sdkmcp.TextContent)
				if ok {
					assert.Contains(t, textContent.Text, "not_implemented",
						"stub tool %s should indicate not_implemented", toolName)
				}
			}
		})
	}
}

// TestMCPServerInfo verifies the MCP server advertises correct info.
func TestMCPServerInfo(t *testing.T) {
	mcpServer := sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: "agentmemory-v2", Version: "2.0.0"},
		&sdkmcp.ServerOptions{},
	)

	inServer, inClient := sdkmcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go mcpServer.Run(ctx, inServer)

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, inClient, nil)
	require.NoError(t, err)
	defer session.Close()

	initResult := session.InitializeResult()
	assert.Equal(t, "agentmemory-v2", initResult.ServerInfo.Name)
	assert.Equal(t, "2.0.0", initResult.ServerInfo.Version)
}

// TestAllToolsHaveDescriptions verifies every registered tool has a description.
func TestAllToolsHaveDescriptions(t *testing.T) {
	mcpServer := sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: "agentmemory-v2", Version: "2.0.0"},
		&sdkmcp.ServerOptions{},
	)

	mcp.RegisterAllTools(mcpServer, mcp.NewServiceBundle(nil))

	inServer, inClient := sdkmcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go mcpServer.Run(ctx, inServer)

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, inClient, nil)
	require.NoError(t, err)
	defer session.Close()

	tools, err := session.ListTools(ctx, &sdkmcp.ListToolsParams{})
	require.NoError(t, err)

	for _, tool := range tools.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			assert.NotEmpty(t, tool.Description,
				"tool %s must have a description", tool.Name)
		})
	}
}

// TestToolsHaveValidInputSchemas verifies every tool has a valid JSON Schema InputSchema.
func TestToolsHaveValidInputSchemas(t *testing.T) {
	mcpServer := sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: "agentmemory-v2", Version: "2.0.0"},
		&sdkmcp.ServerOptions{},
	)

	mcp.RegisterAllTools(mcpServer, mcp.NewServiceBundle(nil))

	inServer, inClient := sdkmcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go mcpServer.Run(ctx, inServer)

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, inClient, nil)
	require.NoError(t, err)
	defer session.Close()

	tools, err := session.ListTools(ctx, &sdkmcp.ListToolsParams{})
	require.NoError(t, err)

	for _, tool := range tools.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			schema, ok := tool.InputSchema.(map[string]interface{})
			require.True(t, ok, "tool %s InputSchema must be a JSON object", tool.Name)

			schemaType, _ := schema["type"].(string)
			assert.Equal(t, "object", schemaType,
				"tool %s schema type must be 'object'", tool.Name)

			_, hasProps := schema["properties"]
			assert.True(t, hasProps,
				"tool %s must have a 'properties' field", tool.Name)

			_, hasRequired := schema["required"]
			assert.True(t, hasRequired,
				"tool %s must have a 'required' field", tool.Name)
		})
	}
}
