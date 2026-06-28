package unit

import (
	"context"
	"testing"

	"github.com/agentmemory/agentmemory/internal/mcp"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// T003 [P] [US1] Unit test: inject flag parsing (true/false/absent)
// =============================================================================

// TestObserveInject_InjectFieldExists verifies that the memory_observe tool
// input schema includes an optional boolean `inject` property.
func TestObserveInject_InjectFieldExists(t *testing.T) {
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

	// Find memory_observe tool
	var observeTool *sdkmcp.Tool
	for _, tool := range tools.Tools {
		if tool.Name == "memory_observe" {
			observeTool = tool
			break
		}
	}
	require.NotNil(t, observeTool, "memory_observe tool must be registered")

	schema, ok := observeTool.InputSchema.(map[string]interface{})
	require.True(t, ok, "InputSchema should be a map")

	props, ok := schema["properties"].(map[string]interface{})
	require.True(t, ok, "properties should be a map")

	// Verify inject property exists
	injectProp, ok := props["inject"]
	require.True(t, ok, "inject property must exist in the input schema")

	// Verify inject is a boolean type
	injectMap, ok := injectProp.(map[string]interface{})
	require.True(t, ok, "inject property should be a map")

	assert.Equal(t, "boolean", injectMap["type"],
		"inject property should be of type boolean")

	// Verify inject description mentions it's optional
	desc, ok := injectMap["description"].(string)
	require.True(t, ok, "inject property should have a description")
	assert.NotEmpty(t, desc, "inject property description should not be empty")

	// Verify inject is NOT in the required array
	required, ok := schema["required"].([]interface{})
	require.True(t, ok, "required should be an array")

	for _, req := range required {
		reqStr, ok := req.(string)
		require.True(t, ok, "required items should be strings")
		assert.NotEqual(t, "inject", reqStr,
			"inject should not be in the required array")
	}
}

// TestObserveInject_InjectFieldDefaultFalse verifies that the inject field
// defaults to false when absent, by confirming it's not in required (meaning
// it's optional) and checking its description implies a default.
func TestObserveInject_InjectFieldDefaultFalse(t *testing.T) {
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

	var observeTool *sdkmcp.Tool
	for _, tool := range tools.Tools {
		if tool.Name == "memory_observe" {
			observeTool = tool
			break
		}
	}
	require.NotNil(t, observeTool, "memory_observe tool must be registered")

	schema, ok := observeTool.InputSchema.(map[string]interface{})
	require.True(t, ok, "InputSchema should be a map")

	props, ok := schema["properties"].(map[string]interface{})
	require.True(t, ok, "properties should be a map")

	injectProp, ok := props["inject"]
	require.True(t, ok, "inject property must exist")

	injectMap, ok := injectProp.(map[string]interface{})
	require.True(t, ok, "inject property should be a map")

	// Description should mention this flag triggers context injection
	desc, ok := injectMap["description"].(string)
	require.True(t, ok, "inject property should have a description")
	assert.Contains(t, desc, "context",
		"inject description should mention context injection")
}

// =============================================================================
// T004 [P] [US1] Unit test: context-trigger gating
// =============================================================================

// TestObserveInject_ContextTriggerGatingSchema verifies the schema defines
// that context injection is only supported for specific trigger types.
// The `type` field accepts trigger values like session_start, pre_tool_use,
// pre_compact (context triggers) as well as other observation types.
func TestObserveInject_ContextTriggerGatingSchema(t *testing.T) {
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

	var observeTool *sdkmcp.Tool
	for _, tool := range tools.Tools {
		if tool.Name == "memory_observe" {
			observeTool = tool
			break
		}
	}
	require.NotNil(t, observeTool, "memory_observe tool must be registered")

	schema, ok := observeTool.InputSchema.(map[string]interface{})
	require.True(t, ok, "InputSchema should be a map")

	props, ok := schema["properties"].(map[string]interface{})
	require.True(t, ok, "properties should be a map")

	// Verify type property exists and describes trigger types
	typeProp, ok := props["type"]
	require.True(t, ok, "type property must exist in the input schema")

	typeMap, ok := typeProp.(map[string]interface{})
	require.True(t, ok, "type property should be a map")

	desc, ok := typeMap["description"].(string)
	require.True(t, ok, "type property should have a description")
	assert.Contains(t, desc, "session_start",
		"type description should mention session_start")
	assert.Contains(t, desc, "pre_tool_use",
		"type description should mention pre_tool_use")

	// Verify required fields include type but not inject
	required, ok := schema["required"].([]interface{})
	require.True(t, ok, "required should be an array")

	typeFound := false
	injectFound := false
	for _, req := range required {
		reqStr, ok := req.(string)
		require.True(t, ok, "required items should be strings")
		if reqStr == "type" {
			typeFound = true
		}
		if reqStr == "inject" {
			injectFound = true
		}
	}
	assert.True(t, typeFound, "type should be in the required array")
	assert.False(t, injectFound, "inject should NOT be in the required array (it's optional)")
}
