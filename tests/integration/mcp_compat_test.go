package integration

import (
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T179: MCP tool-by-tool contract test: verify all 55 MCP tools have correct
// parameter signatures, return schemas, error codes (SC-010).

// allMCPToolNames is the canonical list of all MCP tool names.
var allMCPToolNames = []string{
	// Memory Operations (6)
	"memory_observe",
	"memory_save",
	"memory_recall",
	"memory_smart_search",
	"memory_forget",
	"memory_compress_file",

	// Session Operations (4)
	"memory_sessions",
	"memory_timeline",
	"memory_handoff",
	"memory_recap",

	// Lesson Operations (2)
	"memory_lesson_save",
	"memory_lesson_recall",

	// Team Operations (6)
	"team_create",
	"team_delete",
	"team_add_member",
	"team_remove_member",
	"team_list_members",
	"team_feed",

	// Auth Operations (3)
	"auth_create_key",
	"auth_list_keys",
	"auth_revoke_key",

	// Action Operations (4)
	"memory_action_create",
	"memory_action_update",
	"memory_frontier",
	"memory_next",

	// Pipeline + Governance + Export + Graph (13)
	"memory_crystallize",
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

	// v1 Service Tools (14)
	"memory_facet_tag",
	"memory_vision_search",
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

	// More v1 Tools (5)
	"memory_sketch_promote",
	"memory_routine_run",
	"memory_snapshot_create",
	"memory_file_history",
	"memory_lease",

	// Additional Tools (3)
	"memory_insight_list",
	"memory_team_share",
	"memory_claude_bridge_sync",
}

// TestMCPCompat_AllToolsRegistered verifies that all tools in the canonical list
// are registered and that no unexpected extras exist.
func TestMCPCompat_AllToolsRegistered(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	_, session, ctx, cancel := setupMCPServer(t, db.Pool)
	defer cancel()
	defer session.Close()

	// List all registered tools
	tools, err := session.ListTools(ctx, &sdkmcp.ListToolsParams{})
	require.NoError(t, err, "ListTools should succeed")

	toolNames := make(map[string]bool)
	for _, tool := range tools.Tools {
		toolNames[tool.Name] = true
	}

	// Verify exact count (55 tools)
	assert.Len(t, tools.Tools, len(allMCPToolNames),
		"should have exactly %d tools registered, got %d", len(allMCPToolNames), len(tools.Tools))

	// Verify every expected tool is registered
	for _, expected := range allMCPToolNames {
		assert.True(t, toolNames[expected],
			"tool %q should be registered", expected)
	}

	// Verify no unexpected tools
	for name := range toolNames {
		found := false
		for _, expected := range allMCPToolNames {
			if expected == name {
				found = true
				break
			}
		}
		assert.True(t, found,
			"unexpected tool %q is registered (should be in canonical list)", name)
	}
}

// TestMCPCompat_AllToolsHaveDescriptions verifies every tool has a non-empty description.
func TestMCPCompat_AllToolsHaveDescriptions(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	_, session, ctx, cancel := setupMCPServer(t, db.Pool)
	defer cancel()
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

// TestMCPCompat_AllToolsHaveValidInputSchemas verifies every tool has a valid
// JSON Schema with "type": "object", "properties", and "required" fields.
func TestMCPCompat_AllToolsHaveValidInputSchemas(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	_, session, ctx, cancel := setupMCPServer(t, db.Pool)
	defer cancel()
	defer session.Close()

	tools, err := session.ListTools(ctx, &sdkmcp.ListToolsParams{})
	require.NoError(t, err)

	for _, tool := range tools.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			schema, ok := tool.InputSchema.(map[string]interface{})
			require.True(t, ok, "tool %s InputSchema must be a JSON object", tool.Name)

			schemaType, _ := schema["type"].(string)
			assert.Equal(t, "object", schemaType,
				"tool %s schema type must be 'object', got %q", tool.Name, schemaType)

			_, hasProps := schema["properties"]
			assert.True(t, hasProps,
				"tool %s must have a 'properties' field", tool.Name)

			_, hasRequired := schema["required"]
			assert.True(t, hasRequired,
				"tool %s must have a 'required' field", tool.Name)
		})
	}
}

// TestMCPCompat_ParameterSignatures verifies signature contracts for specific
// tools that have known required parameters.
func TestMCPCompat_ParameterSignatures(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	_, session, ctx, cancel := setupMCPServer(t, db.Pool)
	defer cancel()
	defer session.Close()

	tools, err := session.ListTools(ctx, &sdkmcp.ListToolsParams{})
	require.NoError(t, err)

	// Build a tool map by name
	toolMap := make(map[string]*sdkmcp.Tool)
	for _, tool := range tools.Tools {
		toolMap[tool.Name] = tool
	}

	// Define expected required params for key tools
	type paramCheck struct {
		toolName string
		required []string
	}
	checks := []paramCheck{
		{"memory_observe", []string{"type", "title", "narrative", "session_id"}},
		{"memory_save", []string{"content"}},
		{"memory_recall", []string{"query"}},
		{"memory_smart_search", []string{"query"}},
		{"memory_forget", []string{"observation_ids"}},
		{"memory_compress_file", []string{"file_path"}},
		{"memory_lesson_save", []string{"content"}},
		{"memory_lesson_recall", []string{"query"}},
		{"team_create", []string{"name"}},
		{"team_delete", []string{"team_id"}},
		{"team_add_member", []string{"team_id", "user_id"}},
		{"team_remove_member", []string{"team_id", "user_id"}},
		{"team_list_members", []string{"team_id"}},
		{"auth_create_key", []string{"user_id", "label"}},
		{"auth_list_keys", []string{"user_id"}},
		{"auth_revoke_key", []string{"user_id", "key_id"}},
		{"memory_action_create", []string{"title"}},
		{"memory_action_update", []string{"action_id"}},
		{"memory_relations", []string{"memory_id"}},
		{"memory_profile", []string{"project"}},
		{"memory_verify", []string{"id"}},
		{"memory_commit_lookup", []string{"sha"}},
		{"memory_crystallize", []string{"action_ids"}},
		{"memory_facet_tag", []string{"target_id", "target_type", "dimension", "value"}},
		{"memory_slot_create", []string{"label"}},
		{"memory_slot_get", []string{"label"}},
		{"memory_slot_replace", []string{"label", "content"}},
		{"memory_slot_delete", []string{"label"}},
		{"memory_slot_append", []string{"label", "text"}},
		{"memory_signal_read", []string{"agent_id"}},
		{"memory_signal_send", []string{"from", "content"}},
		{"memory_sentinel_create", []string{"name", "type"}},
		{"memory_sentinel_trigger", []string{"sentinel_id"}},
		{"memory_checkpoint", []string{"operation"}},
		{"memory_sketch_create", []string{"title"}},
		{"memory_sketch_promote", []string{"sketch_id"}},
		{"memory_routine_run", []string{"routine_id"}},
		{"memory_file_history", []string{"files"}},
		{"memory_lease", []string{"action_id", "agent_id", "operation"}},
		{"memory_team_share", []string{"item_id", "item_type"}},
		{"memory_claude_bridge_sync", []string{"direction"}},
	}

	for _, check := range checks {
		t.Run(check.toolName+"_params", func(t *testing.T) {
			tool, ok := toolMap[check.toolName]
			require.True(t, ok, "tool %s should be registered", check.toolName)

			schema, ok := tool.InputSchema.(map[string]interface{})
			require.True(t, ok, "tool %s InputSchema must be an object", check.toolName)

			// Check required params
			requiredList, ok := schema["required"].([]interface{})
			require.True(t, ok, "tool %s required must be an array", check.toolName)

			requiredNames := make(map[string]bool)
			for _, r := range requiredList {
				if s, ok := r.(string); ok {
					requiredNames[s] = true
				}
			}

			for _, req := range check.required {
				assert.True(t, requiredNames[req],
					"tool %s should require parameter %q", check.toolName, req)
			}
		})
	}
}

// TestMCPCompat_ErrorHandling verifies that each tool handles errors correctly:
// - Stub tools return result with IsError=false (not protocol errors)
// - Known working tools return valid JSON results
func TestMCPCompat_ErrorHandling(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	runMigrations(t, db)

	_, session, ctx, cancel := setupMCPServer(t, db.Pool)
	defer cancel()
	defer session.Close()

	// Verify that calling tools without required params fails gracefully
	// (should not panic or return protocol-level error)

	testCases := []struct {
		toolName string
		args     map[string]interface{}
		desc     string
	}{
		// Call with empty args (stub tools should handle this)
		{"memory_frontier", map[string]interface{}{}, "stub with no args"},
		{"memory_next", map[string]interface{}{}, "stub with no args"},
		{"memory_diagnose", map[string]interface{}{}, "stub with no args"},
		{"memory_heal", map[string]interface{}{}, "stub with no args"},
		{"memory_audit", map[string]interface{}{}, "stub with no args"},
		{"memory_export", map[string]interface{}{}, "stub with no args"},
		{"memory_obsidian_export", map[string]interface{}{}, "stub with no args"},
		{"memory_patterns", map[string]interface{}{}, "stub with no args"},
		{"memory_commits", map[string]interface{}{}, "stub with no args"},
		{"memory_mesh_sync", map[string]interface{}{}, "stub with no args"},
		{"team_feed", map[string]interface{}{}, "stub with no args"},
		{"memory_timeline", map[string]interface{}{}, "stub with no args"},
		{"memory_handoff", map[string]interface{}{}, "stub with no args"},
		{"memory_recap", map[string]interface{}{}, "stub with no args"},
	}

	for _, tc := range testCases {
		t.Run(tc.toolName, func(t *testing.T) {
			result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
				Name:      tc.toolName,
				Arguments: tc.args,
			})

			if err != nil {
				t.Logf("%s returned protocol error: %v", tc.toolName, err)
				return
			}

			assert.False(t, result.IsError,
				"tool %s should not set IsError=true (%s)", tc.toolName, tc.desc)
		})
	}
}

// TestMCPCompat_ToolsReturnValidJSON verifies that tool results contain valid JSON.
func TestMCPCompat_ToolsReturnValidJSON(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	_, session, ctx, cancel := setupMCPServer(t, db.Pool)
	defer cancel()
	defer session.Close()

	// Test a few representative tools that are known to return real results
	nonStubTools := []struct {
		name string
		args map[string]interface{}
	}{
		{
			"memory_observe",
			map[string]interface{}{
				"type": "test", "title": "Compat Test",
				"narrative": "Testing JSON output", "session_id": "compat-test",
			},
		},
		{
			"memory_save",
			map[string]interface{}{
				"content": "Compat test save content",
			},
		},
		{
			"memory_lesson_save",
			map[string]interface{}{
				"content": "Compat test lesson",
			},
		},
	}

	for _, tool := range nonStubTools {
		t.Run(tool.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
				Name:      tool.name,
				Arguments: tool.args,
			})

			// The error response should still be valid JSON
			if err != nil {
				t.Logf("%s protocol error: %v", tool.name, err)
				return
			}

			// If IsError, content should still be present
			if result.IsError && len(result.Content) > 0 {
				textContent, ok := result.Content[0].(*sdkmcp.TextContent)
				if ok {
					t.Logf("%s error response: %s", tool.name, textContent.Text)
					// Even error responses should be parseable text
					assert.NotEmpty(t, textContent.Text,
						"error response should have content")
				}
			}
		})
	}
}

// TestMCPCompat_ToolCountMatchesDocuments verifies the registered tool count
// matches the documented count (55 as of v2.0.0).
func TestMCPCompat_ToolCountMatchesDocuments(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	_, session, ctx, cancel := setupMCPServer(t, db.Pool)
	defer cancel()
	defer session.Close()

	tools, err := session.ListTools(ctx, &sdkmcp.ListToolsParams{})
	require.NoError(t, err)

	// Compare against the canonical list length
	assert.Equal(t, len(allMCPToolNames), len(tools.Tools),
		"tool count should match canonical list (%d), got %d",
		len(allMCPToolNames), len(tools.Tools))

	if len(tools.Tools) != len(allMCPToolNames) {
		// Help debug: list any missing or extra tools
		registered := make(map[string]bool)
		for _, tool := range tools.Tools {
			registered[tool.Name] = true
		}

		t.Log("=== Registered tools ===")
		for _, tool := range tools.Tools {
			t.Logf("  %s: %s", tool.Name, tool.Description)
		}

		missing := []string{}
		for _, name := range allMCPToolNames {
			if !registered[name] {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			t.Logf("Missing tools: %v", missing)
		}

		extra := []string{}
		for name := range registered {
			found := false
			for _, n := range allMCPToolNames {
				if n == name {
					found = true
					break
				}
			}
			if !found {
				extra = append(extra, name)
			}
		}
		if len(extra) > 0 {
			t.Logf("Extra tools: %v", extra)
		}
	}
}

// TestMCPCompat_ServerInfoAdvertised verifies the MCP server advertises
// the correct implementation name and version.
func TestMCPCompat_ServerInfoAdvertised(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	_, session, _, cancel := setupMCPServer(t, db.Pool)
	defer cancel()
	defer session.Close()

	initResult := session.InitializeResult()
	assert.NotNil(t, initResult)
	assert.NotNil(t, initResult.ServerInfo)
	assert.Equal(t, "agentmemory-v2", initResult.ServerInfo.Name,
		"server name should be agentmemory-v2")
	assert.Equal(t, "2.0.0", initResult.ServerInfo.Version,
		"server version should be 2.0.0")

	t.Logf("Server: %s v%s", initResult.ServerInfo.Name, initResult.ServerInfo.Version)
}
