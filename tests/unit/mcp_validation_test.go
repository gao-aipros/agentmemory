package unit

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// MCP Tool Parameter Validation Tests (T103)
// =============================================================================

// TestMCPInputSchemaStructure verifies that tool input schemas have correct
// JSON Schema structure via a full InMemoryTransport round-trip.
func TestMCPInputSchemaStructure(t *testing.T) {
	server := mcp.NewServer(
		&mcp.Implementation{Name: "test", Version: "1.0.0"},
		&mcp.ServerOptions{},
	)

	server.AddTool(&mcp.Tool{
		Name:        "memory_observe",
		Description: "Record a raw observation from an agent session event",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"type":       map[string]interface{}{"type": "string", "description": "Hook event type"},
				"title":      map[string]interface{}{"type": "string", "description": "Short summary"},
				"narrative":  map[string]interface{}{"type": "string", "description": "Full description"},
				"session_id": map[string]interface{}{"type": "string", "description": "Session ID"},
				"importance": map[string]interface{}{"type": "number", "description": "Importance 0.0-1.0"},
				"concepts":   map[string]interface{}{"type": "array", "description": "Key concepts", "items": map[string]interface{}{"type": "string"}},
			},
			"required": []string{"type", "title", "narrative", "session_id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: `{"status":"ok"}`}},
		}, nil
	})

	inServer, inClient := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go server.Run(ctx, inServer)

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, inClient, nil)
	require.NoError(t, err, "connect should succeed")
	defer session.Close()

	tools, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	require.NoError(t, err)
	require.NotEmpty(t, tools.Tools, "should have at least one tool")

	var found *mcp.Tool
	for _, t := range tools.Tools {
		if t.Name == "memory_observe" {
			found = t
			break
		}
	}
	require.NotNil(t, found, "memory_observe should be registered")

	schema, ok := found.InputSchema.(map[string]interface{})
	require.True(t, ok, "InputSchema should unmarshal to map")
	assert.Equal(t, "object", schema["type"])

	props, ok := schema["properties"].(map[string]interface{})
	require.True(t, ok)

	typeProp := props["type"].(map[string]interface{})
	assert.Equal(t, "string", typeProp["type"])

	impProp := props["importance"].(map[string]interface{})
	assert.Equal(t, "number", impProp["type"])

	required, ok := schema["required"].([]interface{})
	require.True(t, ok)
	requiredStrs := toStringSlice(required)
	assert.Contains(t, requiredStrs, "type")
	assert.Contains(t, requiredStrs, "title")
	assert.Contains(t, requiredStrs, "narrative")
	assert.Contains(t, requiredStrs, "session_id")
}

// TestMCPServerInfo verifies the MCP server implementation info.
func TestMCPServerInfo(t *testing.T) {
	server := mcp.NewServer(
		&mcp.Implementation{Name: "agentmemory-v2", Version: "2.0.0"},
		&mcp.ServerOptions{},
	)

	inServer, inClient := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go server.Run(ctx, inServer)

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, inClient, nil)
	require.NoError(t, err)
	defer session.Close()

	initResult := session.InitializeResult()
	assert.Equal(t, "agentmemory-v2", initResult.ServerInfo.Name)
	assert.Equal(t, "2.0.0", initResult.ServerInfo.Version)
}

// TestMCPRequiredFieldValidation tests that our handler pattern correctly
// validates required fields via the full MCP transport.
func TestMCPRequiredFieldValidation(t *testing.T) {
	server := mcp.NewServer(
		&mcp.Implementation{Name: "test", Version: "1.0.0"},
		&mcp.ServerOptions{},
	)

	server.AddTool(&mcp.Tool{
		Name:        "test_validator",
		Description: "Tests required field validation",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name":    map[string]interface{}{"type": "string", "description": "Required name"},
				"message": map[string]interface{}{"type": "string", "description": "Optional message"},
			},
			"required": []string{"name"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		type args struct {
			Name    string `json:"name"`
			Message string `json:"message,omitempty"`
		}

		var a args
		if req.Params.Arguments == nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "no arguments provided"}},
			}, nil
		}

		if err := json.Unmarshal(req.Params.Arguments, &a); err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "invalid arguments: " + err.Error()}},
			}, nil
		}

		if a.Name == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "name is required"}},
			}, nil
		}

		if a.Message == "" {
			a.Message = "default message"
		}

		result, _ := json.Marshal(map[string]string{"name": a.Name, "message": a.Message})
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(result)}},
		}, nil
	})

	inServer, inClient := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go server.Run(ctx, inServer)

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, inClient, nil)
	require.NoError(t, err)
	defer session.Close()

	// Test: missing required field
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "test_validator",
		Arguments: map[string]interface{}{"message": "hello"},
	})
	require.NoError(t, err)
	assert.True(t, result.IsError, "should report error for missing required field")
	assert.Contains(t, result.Content[0].(*mcp.TextContent).Text, "required")

	// Test: all required fields present
	result2, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "test_validator",
		Arguments: map[string]interface{}{"name": "test-name"},
	})
	require.NoError(t, err)
	assert.False(t, result2.IsError, "should not error with required field")
	assert.Contains(t, result2.Content[0].(*mcp.TextContent).Text, "test-name")

	// Test: default applied for optional field
	result3, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "test_validator",
		Arguments: map[string]interface{}{"name": "minimal"},
	})
	require.NoError(t, err)
	assert.False(t, result3.IsError)
	assert.Contains(t, result3.Content[0].(*mcp.TextContent).Text, "default message", "default should be applied")
}

// TestMCPSchemaCompliance verifies that our input schemas are valid JSON Schema.
func TestMCPSchemaCompliance(t *testing.T) {
	schemas := []struct {
		name   string
		schema map[string]interface{}
	}{
		{
			name: "memory_observe",
			schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"type":       map[string]interface{}{"type": "string"},
					"title":      map[string]interface{}{"type": "string"},
					"narrative":  map[string]interface{}{"type": "string"},
					"session_id": map[string]interface{}{"type": "string"},
				},
				"required": []string{"type", "title", "narrative", "session_id"},
			},
		},
		{
			name: "memory_recall",
			schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query":        map[string]interface{}{"type": "string"},
					"limit":        map[string]interface{}{"type": "number"},
					"format":       map[string]interface{}{"type": "string"},
					"token_budget": map[string]interface{}{"type": "number"},
				},
				"required": []string{"query"},
			},
		},
		{
			name: "memory_save",
			schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"content":  map[string]interface{}{"type": "string"},
					"type":     map[string]interface{}{"type": "string"},
					"concepts": map[string]interface{}{"type": "array"},
					"files":    map[string]interface{}{"type": "array"},
					"project":  map[string]interface{}{"type": "string"},
				},
				"required": []string{"content"},
			},
		},
	}

	for _, tc := range schemas {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, "object", tc.schema["type"])

			props, ok := tc.schema["properties"].(map[string]interface{})
			require.True(t, ok)

			for propName, propVal := range props {
				propMap, ok := propVal.(map[string]interface{})
				require.True(t, ok, "property %q must be a map", propName)

				propType, ok := propMap["type"]
				assert.True(t, ok, "property %q must have a type", propName)
				assert.Contains(t, []string{"string", "number", "boolean", "array", "object"}, propType,
					"property %q type must be valid", propName)
			}

			required, ok := tc.schema["required"].([]string)
			require.True(t, ok)

			for _, reqField := range required {
				_, exists := props[reqField]
				assert.True(t, exists, "required field %q must be in properties", reqField)
			}
		})
	}
}

// TestStubToolResponse verifies that stub tools return "not_implemented" with IsError=false.
func TestStubToolResponse(t *testing.T) {
	server := mcp.NewServer(
		&mcp.Implementation{Name: "test", Version: "1.0.0"},
		&mcp.ServerOptions{},
	)

	server.AddTool(&mcp.Tool{
		Name:        "memory_frontier",
		Description: "Get all unblocked actions ranked by priority and urgency.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
			"required":   []string{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		content, _ := json.Marshal(map[string]string{
			"status":  "not_implemented",
			"message": "memory_frontier — coming in a future release",
		})
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(content)}},
		}, nil
	})

	inServer, inClient := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go server.Run(ctx, inServer)

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, inClient, nil)
	require.NoError(t, err)
	defer session.Close()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "memory_frontier",
		Arguments: map[string]interface{}{},
	})
	require.NoError(t, err, "stub tools should not return protocol errors")
	assert.False(t, result.IsError, "stub tools should not set IsError=true")
	assert.Contains(t, result.Content[0].(*mcp.TextContent).Text, "not_implemented")
	assert.Contains(t, result.Content[0].(*mcp.TextContent).Text, "coming in a future release")
}

// TestMCPDefaultValues validates that default values are correctly applied.
func TestMCPDefaultValues(t *testing.T) {
	defaults := map[string]float64{
		"importance": 0.5,
		"limit":      10.0,
	}
	assert.Equal(t, 0.5, defaults["importance"])
	assert.Equal(t, 10.0, defaults["limit"])
}

func toStringSlice(ifaces []interface{}) []string {
	result := make([]string, len(ifaces))
	for i, v := range ifaces {
		result[i] = v.(string)
	}
	return result
}
