package mcp

import (
	"context"
	"fmt"

	"github.com/agentmemory/agentmemory/internal/auth"
	"github.com/agentmemory/agentmemory/internal/service"
	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerMemoryInjectContext registers the memory_inject_context MCP tool.
// This tool provides a thin wrapper over ContextHookManager methods for
// session_start, pre_tool_use, and pre_compact context injection triggers.
func registerMemoryInjectContext(mcpServer *mcp.Server, svc *ServiceBundle) {
	type args struct {
		Trigger   string   `json:"trigger"`
		FilePaths []string `json:"file_paths,omitempty"`
	}

	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_inject_context",
		Description: "Inject context on lifecycle hooks — session_start (full context), pre_tool_use (file-specific), or pre_compact (condensed).",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"trigger":    stringProp("Context injection trigger: session_start, pre_tool_use, or pre_compact"),
				"file_paths": arrayProp("File paths for pre_tool_use context (optional, ignored for other triggers)"),
			},
			"required": []string{"trigger"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var a args
		if err := parseArguments(req, &a); err != nil {
			return nil, err
		}

		// Validate trigger
		var hookType service.ContextHookType
		switch a.Trigger {
		case "session_start":
			hookType = service.ContextHookSessionStart
		case "pre_tool_use":
			hookType = service.ContextHookPreToolUse
		case "pre_compact":
			hookType = service.ContextHookPreCompact
		default:
			return nil, fmt.Errorf("invalid trigger %q: must be one of: session_start, pre_tool_use, pre_compact", a.Trigger)
		}

		userID := auth.GetUserIDFromContext(ctx)
		if userID == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: "authentication required: no user ID in context"},
				},
			}, nil
		}

		var result *service.ContextHookResult
		switch hookType {
		case service.ContextHookSessionStart:
			result = svc.ContextHooks.TriggerSessionStart(ctx, userID)
		case service.ContextHookPreToolUse:
			result = svc.ContextHooks.TriggerPreToolUse(ctx, userID, a.FilePaths)
		case service.ContextHookPreCompact:
			result = svc.ContextHooks.TriggerPreCompact(ctx, userID)
		default:
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("unhandled hook type: %s — this is a bug, please report", hookType)},
				},
			}, nil
		}

		// Context text is already wrapped in <agentmemory-context> by ApplyBudget
		// in the service layer — no additional wrapping needed.
		contextText := result.ContextText

		return jsonResult(map[string]interface{}{
			"hook_type":    string(result.HookType),
			"context_text": contextText,
			"skipped":      result.Skipped,
			"skip_reason":  result.SkipReason,
		})
	})
}
