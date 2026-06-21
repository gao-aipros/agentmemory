package service

import (
	"context"
	"fmt"
	"log/slog"
)

// ContextHookType identifies which hook is triggering context injection.
type ContextHookType string

const (
	// ContextHookSessionStart injects full context at session start (all 5 buckets).
	ContextHookSessionStart ContextHookType = "session_start"
	// ContextHookPreToolUse injects file-specific context (search on file paths from tool input).
	ContextHookPreToolUse ContextHookType = "pre_tool_use"
	// ContextHookPreCompact injects condensed context (refresh before window compaction).
	ContextHookPreCompact ContextHookType = "pre_compact"
)

// ContextHookResult is the result of a context hook trigger.
type ContextHookResult struct {
	HookType    ContextHookType
	ContextText string
	Skipped     bool
	SkipReason  string
}

// ContextHookManager manages the 3 context injection triggers:
// SessionStart, PreToolUse, and PreCompact.
type ContextHookManager struct {
	ctxSvc  *ContextService
	gate    *ContextGate
}

// NewContextHookManager creates a new ContextHookManager.
func NewContextHookManager(ctxSvc *ContextService, gate *ContextGate) *ContextHookManager {
	return &ContextHookManager{
		ctxSvc: ctxSvc,
		gate:   gate,
	}
}

// TriggerSessionStart injects full context from all 5 buckets.
// This is the most comprehensive context injection.
func (m *ContextHookManager) TriggerSessionStart(ctx context.Context, userID string) *ContextHookResult {
	if !m.gate.IsEnabled() {
		return &ContextHookResult{
			HookType:   ContextHookSessionStart,
			Skipped:    true,
			SkipReason: "context injection disabled via AGENTMEMORY_INJECT_CONTEXT",
		}
	}

	assembled, err := m.ctxSvc.AssembleContext(ctx, userID)
	if err != nil {
		slog.Warn("failed to assemble context for session_start", "error", err)
		return &ContextHookResult{
			HookType:   ContextHookSessionStart,
			Skipped:    true,
			SkipReason: "assembly failed: " + err.Error(),
		}
	}

	budget := DefaultContextBudget()
	contextText := ApplyBudget(assembled, budget)

	slog.Info("injected context for session_start",
		"tokens", EstimateTokens(contextText),
		"buckets_filled", countFilledBuckets(assembled))

	return &ContextHookResult{
		HookType:    ContextHookSessionStart,
		ContextText: contextText,
	}
}

// TriggerPreToolUse injects file-specific context based on tool input.
// Searches on file paths found in the tool input to find relevant observations.
func (m *ContextHookManager) TriggerPreToolUse(ctx context.Context, userID string, filePaths []string) *ContextHookResult {
	if !m.gate.IsEnabled() {
		return &ContextHookResult{
			HookType:   ContextHookPreToolUse,
			Skipped:    true,
			SkipReason: "context injection disabled via AGENTMEMORY_INJECT_CONTEXT",
		}
	}

	if len(filePaths) == 0 {
		return &ContextHookResult{
			HookType:   ContextHookPreToolUse,
			Skipped:    true,
			SkipReason: "no file paths in tool input",
		}
	}

	// Search for observations related to these file paths
	// For now, use the first file path as a search query
	query := filePaths[0]
	results, err := m.ctxSvc.searchSvc.HybridSearch(ctx, query, 3)
	if err != nil {
		slog.Warn("pre_tool_use context search failed", "error", err)
		return &ContextHookResult{
			HookType:   ContextHookPreToolUse,
			Skipped:    true,
			SkipReason: "search failed: " + err.Error(),
		}
	}

	// Build minimal context from search results
	assembled := &AssembledContext{
		Observations: "",
	}
	var parts []string
	for _, r := range results {
		parts = append(parts, fmt.Sprintf("[%s] file: %s", r.ID, truncate(r.Narrative, 150)))
	}
	assembled.Observations = JoinContextRefs(parts)

	budget := NewContextBudget(400) // Smaller budget for tool-use context
	contextText := ApplyBudget(assembled, budget)

	return &ContextHookResult{
		HookType:    ContextHookPreToolUse,
		ContextText: contextText,
	}
}

// TriggerPreCompact injects condensed context for refresh before window compaction.
// Provides a subset of the full context: graph + lessons only.
func (m *ContextHookManager) TriggerPreCompact(ctx context.Context, userID string) *ContextHookResult {
	if !m.gate.IsEnabled() {
		return &ContextHookResult{
			HookType:   ContextHookPreCompact,
			Skipped:    true,
			SkipReason: "context injection disabled via AGENTMEMORY_INJECT_CONTEXT",
		}
	}

	assembled, err := m.ctxSvc.AssembleContext(ctx, userID)
	if err != nil {
		slog.Warn("failed to assemble context for pre_compact", "error", err)
		return &ContextHookResult{
			HookType:   ContextHookPreCompact,
			Skipped:    true,
			SkipReason: "assembly failed: " + err.Error(),
		}
	}

	// Condensed: only graph and lessons
	condensed := &AssembledContext{
		Graph:         assembled.Graph,
		Lessons:       assembled.Lessons,
		WorkingMemory: assembled.WorkingMemory,
	}

	budget := NewContextBudget(600) // Smaller budget for compact refresh
	contextText := ApplyBudget(condensed, budget)

	slog.Info("injected context for pre_compact",
		"tokens", EstimateTokens(contextText))

	return &ContextHookResult{
		HookType:    ContextHookPreCompact,
		ContextText: contextText,
	}
}

// countFilledBuckets counts how many context buckets have content.
func countFilledBuckets(a *AssembledContext) int {
	count := 0
	if a.Graph != "" {
		count++
	}
	if a.Lessons != "" {
		count++
	}
	if a.Observations != "" {
		count++
	}
	if a.Recap != "" {
		count++
	}
	if a.WorkingMemory != "" {
		count++
	}
	return count
}
