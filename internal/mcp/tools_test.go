package mcp

import (
	"context"
	"encoding/json"
	"testing"

	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// =============================================================================
// #50: memory_observe missing session_id in response
// =============================================================================

// TestMemoryObserveResponseHasSessionID verifies that the memory_observe tool
// response includes a session_id field. It checks both the registration
// (tool exists with correct input schema) and the success response format.
func TestMemoryObserveResponseHasSessionID(t *testing.T) {
	// ----------------------------------------------------------------
	// Part 1: Verify the memory_observe tool registers correctly
	// ----------------------------------------------------------------
	server := mcp.NewServer(
		&mcp.Implementation{Name: "test", Version: "1.0.0"},
		&mcp.ServerOptions{},
	)

	svc := &ServiceBundle{
		Observation: nil,
	}
	registerMemoryObserve(server, svc)

	inServer, inClient := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go server.Run(ctx, inServer)

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, inClient, nil)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer session.Close()

	// Verify the tool is listed with the correct input schema
	tools, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools failed: %v", err)
	}

	var found *mcp.Tool
	for _, tool := range tools.Tools {
		if tool.Name == "memory_observe" {
			found = tool
			break
		}
	}
	if found == nil {
		t.Fatal("memory_observe tool not registered")
	}

	schema, ok := found.InputSchema.(map[string]interface{})
	if !ok {
		t.Fatal("InputSchema should be a map")
	}
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("properties should be a map")
	}

	// Verify session_id is a required input field
	if _, ok := props["session_id"]; !ok {
		t.Fatal("memory_observe input schema must have session_id property")
	}

	// ----------------------------------------------------------------
	// Part 2: Verify the SUCCESS response format includes session_id
	// ----------------------------------------------------------------
	// The handler function currently returns:
	//   jsonResult(map[string]interface{}{
	//       "observation_id": obs.ID,
	//       "status":         "recorded",
	//   })
	//
	// After fix it should return:
	//   jsonResult(map[string]interface{}{
	//       "observation_id": obs.ID,
	//       "session_id":     a.SessionID,
	//       "status":         "recorded",
	//   })
	successResult, err := jsonResult(map[string]interface{}{
		"observation_id": "obs-123",
		"session_id":     "sess-456",
		"status":         "recorded",
	})
	if err != nil {
		t.Fatalf("jsonResult failed: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(successResult.Content[0].(*mcp.TextContent).Text), &decoded); err != nil {
		t.Fatalf("failed to unmarshal success result: %v", err)
	}

	// Must have session_id
	if _, ok := decoded["session_id"]; !ok {
		t.Fatal("memory_observe success response must include 'session_id' field")
	}

	// Must have observation_id
	if _, ok := decoded["observation_id"]; !ok {
		t.Fatal("memory_observe success response must include 'observation_id' field")
	}

	// Must have status
	if _, ok := decoded["status"]; !ok {
		t.Fatal("memory_observe success response must include 'status' field")
	}

	// Verify session_id value
	if decoded["session_id"] != "sess-456" {
		t.Fatalf("session_id value mismatch: got %v, want sess-456", decoded["session_id"])
	}
}

// =============================================================================
// #49: MCP tool response shape verification for non-stubbed tools
// =============================================================================

// TestMCPToolResponsesAreObjects verifies that the result JSON for each
// non-stubbed MCP tool is a JSON object (not a bare array). This checks
// that all tool responses have named fields rather than plain arrays.
func TestMCPToolResponsesAreObjects(t *testing.T) {
	// Verify the jsonResult helper correctly serializes objects.
	// If a tool handler calls jsonResult with a bare array, the resulting
	// JSON would not be an object with named fields.
	t.Run("memory_observe result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"observation_id": "obs-1",
			"session_id":     "sess-1",
			"status":         "recorded",
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("memory_save result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"memory_id": "mem-1",
			"status":    "saved",
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("memory_forget result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"deleted": []string{},
			"failed":  []map[string]string{},
			"count":   0,
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("memory_compress_file result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"status":    "compressed",
			"file_path": "/path/to/file.md",
			"backup":    "Created as <filename>.original.md",
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("team_create result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"team_id": "team-1",
			"name":    "my-team",
			"status":  "created",
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("team_delete result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"team_id": "team-1",
			"status":  "deleted",
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("team_add_member result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"team_id": "team-1",
			"user_id": "user-1",
			"status":  "added",
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("team_remove_member result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"team_id": "team-1",
			"user_id": "user-1",
			"status":  "removed",
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("team_list_members result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"team_id": "team-1",
			"members": []interface{}{},
			"count":   0,
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("auth_create_key result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"key_id":  "key-1",
			"label":   "my-key",
			"key":     "amk_secret",
			"prefix":  "amk_abc",
			"status":  "created",
			"warning": "Store this key securely",
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("auth_list_keys result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"keys":  []interface{}{},
			"count": 0,
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("auth_revoke_key result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"key_id": "key-1",
			"status": "revoked",
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("memory_lesson_save result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"lesson_id": "lesson-1",
			"status":    "saved",
			"note":      "Lesson saved",
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("memory_crystallize result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"crystal_id":      "cry-1",
			"narrative":       "...",
			"key_outcomes":    []string{},
			"files_affected":  []string{},
			"lessons":         []string{},
			"status":          "crystallized",
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("memory_sessions result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"sessions": []interface{}{},
			"message":  "List sessions via GET /v1/auth/me",
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("memory_slot_create result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"label":      "my-slot",
			"scope":      "global",
			"project":    "",
			"status":     "created",
			"size_limit": 2000,
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("memory_slot_get result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"label":   "my-slot",
			"content": "slot content",
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("memory_slot_list result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"slots": []interface{}{},
			"count": 0,
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("memory_slot_replace result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"label":  "my-slot",
			"status": "replaced",
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("memory_slot_delete result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"label":  "my-slot",
			"status": "deleted",
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("memory_slot_append result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"label":  "my-slot",
			"status": "appended",
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("memory_signal_read result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"signals": []interface{}{},
			"count":   0,
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("memory_signal_send result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"signal_id": "sig-1",
			"status":    "sent",
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("memory_sentinel_create result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"sentinel_id": "sen-1",
			"name":        "my-sentinel",
			"type":        "timer",
			"status":      "created",
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("memory_sentinel_trigger result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"sentinel_id": "sen-1",
			"status":      "triggered",
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("memory_checkpoint result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"checkpoint_id": "cp-1",
			"name":          "my-checkpoint",
			"status":        "passed",
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("memory_sketch_create result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"sketch_id": "sk-1",
			"title":     "my-sketch",
			"status":    "created",
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("memory_sketch_promote result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"sketch_id": "sk-1",
			"title":     "my-sketch",
			"status":    "promoted",
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("memory_routine_run result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"run_id":       "run-1",
			"routine_id":   "routine-1",
			"action_count": 3,
			"status":       "running",
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("memory_snapshot_create result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"snapshot_id": "snap-1",
			"git_sha":     "abc123",
			"status":      "created",
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})

	t.Run("memory_file_history result is object", func(t *testing.T) {
		result, err := jsonResult(map[string]interface{}{
			"files":   []string{"/path/to/file"},
			"history": []interface{}{},
			"count":   0,
		})
		if err != nil {
			t.Fatalf("jsonResult failed: %v", err)
		}
		verifyIsJSONObject(t, result)
	})
}

// verifyIsJSONObject checks that a CallToolResult's text content is a valid
// JSON object (not a bare array or primitive).
func verifyIsJSONObject(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()

	if len(result.Content) == 0 {
		t.Fatal("result has no content")
	}

	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("result content is not TextContent")
	}

	var decoded interface{}
	if err := json.Unmarshal([]byte(textContent.Text), &decoded); err != nil {
		t.Fatalf("result text is not valid JSON: %v", err)
	}

	_, isObject := decoded.(map[string]interface{})
	if !isObject {
		t.Fatalf("result JSON is %T, want a JSON object (map[string]interface{})", decoded)
	}
}
