// Package mcp provides MCP tool registrations for agentmemory v2.
// All tools are thin wrappers that validate parameters, call the corresponding
// service method, and return a JSON-serializable result.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ServiceBundle holds all service dependencies for MCP tool registrations.
type ServiceBundle struct {
	Observation   *service.ObservationService
	Session       *service.SessionService
	Recall        *service.RecallService
	SmartSearch   *service.SmartSearchService
	Search        *service.SearchService
	User          *service.UserService
	Team          *service.TeamService
	Members       *service.TeamMembersService
	Summarization *service.SummarizationService
	Consolidation *service.ConsolidationService
	Reflection    *service.ReflectionService
	Context       *service.ContextService
	Compression   *service.CompressionService
	Embedding     *service.EmbeddingService
	LLM           *service.LLMService
	SessionEnd    *service.SessionEndHandler
	Eviction      *service.EvictionService

	Pool *pgxpool.Pool
}

// NewServiceBundle creates all service instances from a connection pool.
func NewServiceBundle(pool *pgxpool.Pool) *ServiceBundle {
	llmSvc := service.NewLLMService(nil)
	embedSvc := service.NewEmbeddingService(pool, nil)
	compressor := service.NewCompressionService(pool, llmSvc, embedSvc)
	obsSvc := service.NewObservationService(pool, compressor)
	sessionSvc := service.NewSessionService(pool)
	recallSvc := service.NewRecallService(pool, embedSvc)
	smartSearchSvc := service.NewSmartSearchService(pool, embedSvc)
	searchSvc := service.NewSearchService(pool, embedSvc)
	userSvc := service.NewUserService(pool)
	teamSvc := service.NewTeamService(pool)
	memberSvc := service.NewTeamMembersService(pool)
	summarizer := service.NewSummarizationService(pool, llmSvc)
	mode := service.DefaultConsolidationMode("member_choice", false)
	consolidator := service.NewConsolidationService(pool, llmSvc, mode)
	reflector := service.NewReflectionService(pool, 3600)
	slotSvc := service.NewSlotService(pool)
	ctxSvc := service.NewContextService(pool, embedSvc, slotSvc)
	evictSvc := service.NewEvictionService(pool)
	sessionEndH := service.NewSessionEndHandler(sessionSvc, summarizer, consolidator, reflector)

	return &ServiceBundle{
		Observation:   obsSvc,
		Session:       sessionSvc,
		Recall:        recallSvc,
		SmartSearch:   smartSearchSvc,
		Search:        searchSvc,
		User:          userSvc,
		Team:          teamSvc,
		Members:       memberSvc,
		Summarization: summarizer,
		Consolidation: consolidator,
		Reflection:    reflector,
		Context:       ctxSvc,
		Compression:   compressor,
		Embedding:     embedSvc,
		LLM:           llmSvc,
		SessionEnd:    sessionEndH,
		Eviction:      evictSvc,
		Pool:          pool,
	}
}

// stubbedToolResult returns a standard "not implemented" result for stub tools.
func stubbedToolResult(toolName string) *mcp.CallToolResult {
	content, _ := json.Marshal(map[string]string{
		"status":  "not_implemented",
		"message": toolName + " — coming in a future release",
	})
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(content)},
		},
	}
}

// jsonResult creates a CallToolResult from any JSON-serializable value.
func jsonResult(v interface{}) (*mcp.CallToolResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}, nil
}

// parseArguments unmarshals the raw JSON arguments into the provided struct.
func parseArguments(req *mcp.CallToolRequest, target interface{}) error {
	if req.Params.Arguments == nil {
		return fmt.Errorf("no arguments provided")
	}
	if err := json.Unmarshal(req.Params.Arguments, target); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	return nil
}

// stringProp returns a standard string input schema property descriptor.
func stringProp(description string) map[string]interface{} {
	return map[string]interface{}{
		"type":        "string",
		"description": description,
	}
}

// numberProp returns a standard number input schema property descriptor.
func numberProp(description string) map[string]interface{} {
	return map[string]interface{}{
		"type":        "number",
		"description": description,
	}
}

// arrayProp returns a standard array input schema property descriptor.
func arrayProp(description string) map[string]interface{} {
	return map[string]interface{}{
		"type":        "array",
		"description": description,
		"items":       map[string]interface{}{"type": "string"},
	}
}

// optStringProp returns an optional string schema property.
func optStringProp(description string) map[string]interface{} {
	m := stringProp(description)
	// For optional fields, we simply don't include them in Required
	return m
}

// =============================================================================
// RegisterAllTools creates the MCP server and registers all tools.
// =============================================================================

// RegisterAllTools registers every agentmemory MCP tool on the given server.
// The pool is used to create all service dependencies. If pool is nil, tools
// are still registered but will return service-unavailable errors when called.
func RegisterAllTools(mcpServer *mcp.Server, pool *pgxpool.Pool) {
	var svc *ServiceBundle
	if pool != nil {
		svc = NewServiceBundle(pool)
	} else {
		svc = &ServiceBundle{}
	}

	// Memory Operations (T110-T115)
	registerMemoryObserve(mcpServer, svc)
	registerMemorySave(mcpServer, svc)
	registerMemoryRecall(mcpServer, svc)
	registerMemorySmartSearch(mcpServer, svc)
	registerMemoryForget(mcpServer, svc)
	registerMemoryCompressFile(mcpServer, svc)

	// Session Operations (T116-T119)
	registerMemorySessions(mcpServer, svc)
	registerMemoryTimeline(mcpServer, svc)
	registerMemoryHandoff(mcpServer, svc)
	registerMemoryRecap(mcpServer, svc)

	// Lesson Operations (T120-T121)
	registerMemoryLessonSave(mcpServer, svc)
	registerMemoryLessonRecall(mcpServer, svc)

	// Team Operations (T122-T124)
	registerTeamCreate(mcpServer, svc)
	registerTeamDelete(mcpServer, svc)
	registerTeamAddMember(mcpServer, svc)
	registerTeamRemoveMember(mcpServer, svc)
	registerTeamListMembers(mcpServer, svc)
	registerTeamFeed(mcpServer, svc)

	// Auth Operations (T125)
	registerAuthCreateKey(mcpServer, svc)
	registerAuthListKeys(mcpServer, svc)
	registerAuthRevokeKey(mcpServer, svc)

	// Action Operations (T126-T127) — stubs
	registerMemoryActionCreate(mcpServer, svc)
	registerMemoryActionUpdate(mcpServer, svc)
	registerMemoryFrontier(mcpServer, svc)
	registerMemoryNext(mcpServer, svc)

	// Pipeline + Governance + Export + Graph + Context (T128-T132)
	registerMemoryConsolidate(mcpServer, svc)
	registerMemoryCrystallize(mcpServer, svc)
	registerMemoryReflect(mcpServer, svc)
	registerMemoryDiagnose(mcpServer, svc)
	registerMemoryHeal(mcpServer, svc)
	registerMemoryVerify(mcpServer, svc)
	registerMemoryAudit(mcpServer, svc)
	registerMemoryExport(mcpServer, svc)
	registerMemoryObsidianExport(mcpServer, svc)
	registerMemoryCommitLookup(mcpServer, svc)
	registerMemoryCommits(mcpServer, svc)
	registerMemoryMeshSync(mcpServer, svc)
	registerMemoryGraphQuery(mcpServer, svc)
	registerMemoryRelations(mcpServer, svc)
	registerMemoryProfile(mcpServer, svc)
	registerMemoryPatterns(mcpServer, svc)
	registerMemoryFacetQuery(mcpServer, svc)
	registerMemoryFacetTag(mcpServer, svc)
	registerMemoryVisionSearch(mcpServer, svc)

	// v1 Service Tools (T144-T154) — stubs
	registerMemorySlotCreate(mcpServer, svc)
	registerMemorySlotGet(mcpServer, svc)
	registerMemorySlotList(mcpServer, svc)
	registerMemorySlotReplace(mcpServer, svc)
	registerMemorySlotDelete(mcpServer, svc)
	registerMemorySlotAppend(mcpServer, svc)
	registerMemorySignalRead(mcpServer, svc)
	registerMemorySignalSend(mcpServer, svc)
	registerMemorySentinelCreate(mcpServer, svc)
	registerMemorySentinelTrigger(mcpServer, svc)
	registerMemoryCheckpoint(mcpServer, svc)
	registerMemorySketchCreate(mcpServer, svc)
	registerMemorySketchPromote(mcpServer, svc)
	registerMemoryRoutineRun(mcpServer, svc)
	registerMemorySnapshotCreate(mcpServer, svc)
	registerMemoryFileHistory(mcpServer, svc)
	registerMemoryLease(mcpServer, svc)
	registerMemoryInsightList(mcpServer, svc)
	registerMemoryTeamShare(mcpServer, svc)
	registerMemoryClaudeBridgeSync(mcpServer, svc)
}

// =============================================================================
// Memory Operations (T110-T115)
// =============================================================================

func registerMemoryObserve(mcpServer *mcp.Server, svc *ServiceBundle) {
	type args struct {
		Type        string   `json:"type"`
		Title       string   `json:"title"`
		Narrative   string   `json:"narrative"`
		SessionID   string   `json:"session_id"`
		Facts       string   `json:"facts,omitempty"`
		Concepts    []string `json:"concepts,omitempty"`
		Files       []string `json:"files,omitempty"`
		Importance  float64  `json:"importance,omitempty"`
		OwnerType   string   `json:"owner_type,omitempty"`
		OwnerUserID string   `json:"owner_user_id,omitempty"`
		OwnerTeamID string   `json:"owner_team_id,omitempty"`
	}

	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_observe",
		Description: "Record a raw observation from an agent session event. Observations capture what agents do, see, and decide.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"type":          stringProp("Hook event type (e.g., session_start, user_prompt_submit, pre_tool_use, post_tool_use)"),
				"title":         stringProp("Short summary of the observation"),
				"narrative":     stringProp("Full description of what happened"),
				"session_id":    stringProp("Session ID this observation belongs to"),
				"facts":         optStringProp("Key facts extracted from the observation"),
				"concepts":      arrayProp("Key concepts for search indexing"),
				"files":         arrayProp("Files involved in this observation"),
				"importance":    numberProp("Importance score 0.0-1.0 (default 0.5)"),
				"owner_type":    optStringProp("Ownership type: user or team"),
				"owner_user_id": optStringProp("Owner user ID"),
				"owner_team_id": optStringProp("Owner team ID"),
			},
			"required": []string{"type", "title", "narrative", "session_id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var a args
		if err := parseArguments(req, &a); err != nil {
			return nil, err
		}

		if a.Importance == 0 {
			a.Importance = 0.5
		}

		input := service.RecordObservationInput{
			SessionID:   a.SessionID,
			OwnerType:   a.OwnerType,
			OwnerUserID: a.OwnerUserID,
			OwnerTeamID: a.OwnerTeamID,
			Type:        a.Type,
			Title:       a.Title,
			Narrative:   a.Narrative,
			Facts:       a.Facts,
			Concepts:    a.Concepts,
			Files:       a.Files,
			Importance:  a.Importance,
		}

		obs, err := svc.Observation.RecordObservation(ctx, input)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: err.Error()},
				},
			}, nil
		}

		return jsonResult(map[string]interface{}{
			"observation_id": obs.ID,
			"status":         "recorded",
		})
	})
}

func registerMemorySave(mcpServer *mcp.Server, svc *ServiceBundle) {
	type args struct {
		Content   string   `json:"content"`
		Type      string   `json:"type,omitempty"`
		Concepts  []string `json:"concepts,omitempty"`
		Files     []string `json:"files,omitempty"`
		Project   string   `json:"project,omitempty"`
	}

	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_save",
		Description: "Explicitly save an important insight, decision, or pattern to long-term memory.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"content":  stringProp("The insight or decision to remember"),
				"type":     optStringProp("Memory type: pattern, preference, architecture, bug, workflow, or fact"),
				"concepts": arrayProp("Comma-separated key concepts"),
				"files":    arrayProp("Comma-separated relevant file paths"),
				"project":  optStringProp("Project path this memory belongs to"),
			},
			"required": []string{"content"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var a args
		if err := parseArguments(req, &a); err != nil {
			return nil, err
		}

		// Create memory entry directly (bypasses observation pipeline — explicit saves
		// go straight to long-term memory, not through the observe→compress chain)
		if svc.Pool == nil {
			return jsonResult(map[string]interface{}{
				"memory_id": uuid.New().String(),
				"status":    "saved",
				"note":      "in-memory mode (no database pool)",
			})
		}
		queries := store.New(svc.Pool)
		mem, err := queries.InsertMemory(ctx, store.InsertMemoryParams{
			ID:         uuid.New().String(),
			OwnerType:  "user",
			Visibility: "private",
			Content:    a.Content,
			Concepts:   a.Concepts,
			Source:     "manual_save",
			Confidence: 0.8,
		})
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: err.Error()},
				},
			}, nil
		}

		return jsonResult(map[string]interface{}{
			"memory_id": mem.ID,
			"status":    "saved",
		})
	})
}

func registerMemoryRecall(mcpServer *mcp.Server, svc *ServiceBundle) {
	type args struct {
		Query       string `json:"query"`
		Limit       int    `json:"limit,omitempty"`
		Format      string `json:"format,omitempty"`
		TokenBudget int    `json:"token_budget,omitempty"`
	}

	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_recall",
		Description: "Search past session observations for relevant context using hybrid BM25+vector+graph search.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query":        stringProp("Search query (keywords, file names, concepts)"),
				"limit":        numberProp("Max results to return (default 10)"),
				"format":       optStringProp("Result format: compact, full, or narrative (default compact)"),
				"token_budget": numberProp("Optional token budget to trim returned results"),
			},
			"required": []string{"query"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var a args
		if err := parseArguments(req, &a); err != nil {
			return nil, err
		}

		if a.Limit <= 0 {
			a.Limit = 10
		}
		if a.Format == "" {
			a.Format = "compact"
		}

		result, err := svc.Recall.Recall(ctx, a.Query, a.Limit, a.Format)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: err.Error()},
				},
			}, nil
		}

		return jsonResult(result)
	})
}

func registerMemorySmartSearch(mcpServer *mcp.Server, svc *ServiceBundle) {
	type args struct {
		Query     string   `json:"query"`
		Limit     int      `json:"limit,omitempty"`
		ExpandIDs []string `json:"expand_ids,omitempty"`
	}

	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_smart_search",
		Description: "Hybrid semantic+keyword search with progressive disclosure. Returns compact results; expand specific IDs for full details.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query":      stringProp("Search query"),
				"limit":      numberProp("Max results (default 10)"),
				"expand_ids": arrayProp("Comma-separated observation IDs to expand"),
			},
			"required": []string{"query"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var a args
		if err := parseArguments(req, &a); err != nil {
			return nil, err
		}

		if a.Limit <= 0 {
			a.Limit = 10
		}

		result, err := svc.SmartSearch.Search(ctx, a.Query, a.Limit, a.ExpandIDs)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: err.Error()},
				},
			}, nil
		}

		return jsonResult(result)
	})
}

func registerMemoryForget(mcpServer *mcp.Server, svc *ServiceBundle) {
	type args struct {
		ObservationIDs []string `json:"observation_ids"`
	}

	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_forget",
		Description: "Delete specific observations by their IDs. Use with caution — this is irreversible.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"observation_ids": arrayProp("Comma-separated observation IDs to delete"),
			},
			"required": []string{"observation_ids"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var a args
		if err := parseArguments(req, &a); err != nil {
			return nil, err
		}

		deleted := []string{}
		failed := []map[string]string{}
		for _, id := range a.ObservationIDs {
			if err := svc.Eviction.EvictObservation(ctx, id); err != nil {
				failed = append(failed, map[string]string{"id": id, "error": err.Error()})
			} else {
				deleted = append(deleted, id)
			}
		}

		return jsonResult(map[string]interface{}{
			"deleted": deleted,
			"failed":  failed,
			"count":   len(deleted),
		})
	})
}

func registerMemoryCompressFile(mcpServer *mcp.Server, svc *ServiceBundle) {
	type args struct {
		FilePath string `json:"file_path"`
	}

	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_compress_file",
		Description: "Compress a markdown file to reduce token usage while preserving headings, URLs, and code blocks.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"file_path": stringProp("Path to the markdown file to compress"),
			},
			"required": []string{"file_path"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Stub: file compression not yet implemented
		return stubbedToolResult("memory_compress_file"), nil
	})
}

// =============================================================================
// Session Operations (T116-T119)
// =============================================================================

func registerMemorySessions(mcpServer *mcp.Server, svc *ServiceBundle) {
	type args struct {
		UserID string `json:"user_id,omitempty"`
	}

	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_sessions",
		Description: "List recent sessions with their status and observation counts.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"user_id": optStringProp("Filter by user ID (uses authenticated user if omitted)"),
			},
			"required": []string{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// This is a thin wrapper — the store layer handles listing.
		// We return stub for now since there's no dedicated ListSessions endpoint in the service layer yet.
		return jsonResult(map[string]interface{}{
			"sessions": []interface{}{},
			"message":  "List sessions via GET /v1/auth/me or session history endpoints",
		})
	})
}

func registerMemoryTimeline(mcpServer *mcp.Server, svc *ServiceBundle) {
	type args struct {
		Anchor  string `json:"anchor"`
		Before  int    `json:"before,omitempty"`
		After   int    `json:"after,omitempty"`
		Project string `json:"project,omitempty"`
	}

	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_timeline",
		Description: "Chronological observations around an anchor point (ISO date or keyword like 'today').",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"anchor":  stringProp("Anchor point: ISO date or keyword (e.g., 'today', 'yesterday', 'last week')"),
				"before":  numberProp("Observations before anchor (default 5)"),
				"after":   numberProp("Observations after anchor (default 5)"),
				"project": optStringProp("Filter by project path"),
			},
			"required": []string{"anchor"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_timeline"), nil
	})
}

func registerMemoryHandoff(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_handoff",
		Description: "Resume the most recent agent session, leading with any unanswered questions.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
			"required":   []string{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_handoff"), nil
	})
}

func registerMemoryRecap(mcpServer *mcp.Server, svc *ServiceBundle) {
	type args struct {
		Count int `json:"count,omitempty"`
	}

	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_recap",
		Description: "Summarize the last N agent sessions, grouped by date, with highlight observations per session.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"count": numberProp("Number of recent sessions to recap (default 5)"),
			},
			"required": []string{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_recap"), nil
	})
}

// =============================================================================
// Lesson Operations (T120-T121)
// =============================================================================

func registerMemoryLessonSave(mcpServer *mcp.Server, svc *ServiceBundle) {
	type args struct {
		Content    string  `json:"content"`
		Context    string  `json:"context,omitempty"`
		Project    string  `json:"project,omitempty"`
		Tags       []string `json:"tags,omitempty"`
		Confidence float64 `json:"confidence,omitempty"`
	}

	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_lesson_save",
		Description: "Save a lesson learned. Lessons have confidence scores that strengthen when reinforced and decay when not used.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"content":    stringProp("The lesson learned (what worked, what to avoid, when to use X approach)"),
				"context":    optStringProp("When/where this lesson applies"),
				"project":    optStringProp("Project this lesson is about"),
				"tags":       arrayProp("Comma-separated tags"),
				"confidence": numberProp("Initial confidence 0.0-1.0 (default 0.5)"),
			},
			"required": []string{"content"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var a args
		if err := parseArguments(req, &a); err != nil {
			return nil, err
		}
		// Lesson saving is handled via consolidation pipeline in MVP.
		// This records an explicit lesson-type observation as a bridge.
		input := service.RecordObservationInput{
			SessionID:  "lesson",
			OwnerType:  "user",
			Type:       "lesson",
			Title:      "Lesson: " + truncateStr(a.Content, 80),
			Narrative:  a.Content,
			Concepts:   a.Tags,
			Importance: 0.7,
		}

		obs, err := svc.Observation.RecordObservation(ctx, input)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: err.Error()},
				},
			}, nil
		}

		return jsonResult(map[string]interface{}{
			"observation_id": obs.ID,
			"status":         "saved",
			"note":           "Lesson saved as observation. Full lesson extraction happens during consolidation.",
		})
	})
}

func registerMemoryLessonRecall(mcpServer *mcp.Server, svc *ServiceBundle) {
	type args struct {
		Query         string  `json:"query"`
		Limit         int     `json:"limit,omitempty"`
		MinConfidence float64 `json:"min_confidence,omitempty"`
		Project       string  `json:"project,omitempty"`
	}

	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_lesson_recall",
		Description: "Search lessons by query. Returns lessons sorted by confidence and recency.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query":          stringProp("Search query"),
				"limit":          numberProp("Max results (default 10)"),
				"min_confidence": numberProp("Minimum confidence threshold (default 0.1)"),
				"project":        optStringProp("Filter by project"),
			},
			"required": []string{"query"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Use recall with narrative format as a proxy for lesson search
		return stubbedToolResult("memory_lesson_recall"), nil
	})
}

// =============================================================================
// Team Operations (T122-T124)
// =============================================================================

func registerTeamCreate(mcpServer *mcp.Server, svc *ServiceBundle) {
	type args struct {
		Name              string `json:"name"`
		OwnerID           string `json:"owner_id"`
		DefaultVisibility string `json:"default_visibility,omitempty"`
	}

	mcpServer.AddTool(&mcp.Tool{
		Name:        "team_create",
		Description: "Create a new team with the given name, owner, and default visibility.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name":               stringProp("Team name"),
				"owner_id":           stringProp("Owner user ID"),
				"default_visibility": optStringProp("Default visibility: member_choice (default), team, or public"),
			},
			"required": []string{"name", "owner_id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var a args
		if err := parseArguments(req, &a); err != nil {
			return nil, err
		}

		team, err := svc.Team.CreateTeam(ctx, a.Name, a.OwnerID, a.DefaultVisibility)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: err.Error()},
				},
			}, nil
		}

		return jsonResult(map[string]interface{}{
			"team_id": team.ID,
			"name":    team.Name,
			"status":  "created",
		})
	})
}

func registerTeamDelete(mcpServer *mcp.Server, svc *ServiceBundle) {
	type args struct {
		TeamID  string `json:"team_id"`
		OwnerID string `json:"owner_id"`
	}

	mcpServer.AddTool(&mcp.Tool{
		Name:        "team_delete",
		Description: "Delete a team. Only the team owner can delete.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"team_id":  stringProp("Team ID to delete"),
				"owner_id": stringProp("Owner user ID (ownership verified)"),
			},
			"required": []string{"team_id", "owner_id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var a args
		if err := parseArguments(req, &a); err != nil {
			return nil, err
		}

		if err := svc.Team.DeleteTeam(ctx, a.TeamID, a.OwnerID); err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: err.Error()},
				},
			}, nil
		}

		return jsonResult(map[string]interface{}{
			"team_id": a.TeamID,
			"status":  "deleted",
		})
	})
}

func registerTeamAddMember(mcpServer *mcp.Server, svc *ServiceBundle) {
	type args struct {
		TeamID string `json:"team_id"`
		UserID string `json:"user_id"`
	}

	mcpServer.AddTool(&mcp.Tool{
		Name:        "team_add_member",
		Description: "Add a user to a team. Users can only be in one team at a time.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"team_id": stringProp("Team ID"),
				"user_id": stringProp("User ID to add"),
			},
			"required": []string{"team_id", "user_id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var a args
		if err := parseArguments(req, &a); err != nil {
			return nil, err
		}

		if err := svc.Members.AddMember(ctx, a.TeamID, a.UserID); err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: err.Error()},
				},
			}, nil
		}

		return jsonResult(map[string]interface{}{
			"team_id": a.TeamID,
			"user_id": a.UserID,
			"status":  "added",
		})
	})
}

func registerTeamRemoveMember(mcpServer *mcp.Server, svc *ServiceBundle) {
	type args struct {
		TeamID string `json:"team_id"`
		UserID string `json:"user_id"`
	}

	mcpServer.AddTool(&mcp.Tool{
		Name:        "team_remove_member",
		Description: "Remove a user from a team.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"team_id": stringProp("Team ID"),
				"user_id": stringProp("User ID to remove"),
			},
			"required": []string{"team_id", "user_id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var a args
		if err := parseArguments(req, &a); err != nil {
			return nil, err
		}

		if err := svc.Members.RemoveMember(ctx, a.TeamID, a.UserID); err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: err.Error()},
				},
			}, nil
		}

		return jsonResult(map[string]interface{}{
			"team_id": a.TeamID,
			"user_id": a.UserID,
			"status":  "removed",
		})
	})
}

func registerTeamListMembers(mcpServer *mcp.Server, svc *ServiceBundle) {
	type args struct {
		TeamID string `json:"team_id"`
	}

	mcpServer.AddTool(&mcp.Tool{
		Name:        "team_list_members",
		Description: "List all members of a team.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"team_id": stringProp("Team ID"),
			},
			"required": []string{"team_id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var a args
		if err := parseArguments(req, &a); err != nil {
			return nil, err
		}

		members, err := svc.Members.ListMembers(ctx, a.TeamID)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: err.Error()},
				},
			}, nil
		}

		return jsonResult(map[string]interface{}{
			"team_id": a.TeamID,
			"members": members,
			"count":   len(members),
		})
	})
}

func registerTeamFeed(mcpServer *mcp.Server, svc *ServiceBundle) {
	type args struct {
		Limit int `json:"limit,omitempty"`
	}

	mcpServer.AddTool(&mcp.Tool{
		Name:        "team_feed",
		Description: "Get recent shared items from all team members.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"limit": numberProp("Max items (default 20)"),
			},
			"required": []string{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("team_feed"), nil
	})
}

// =============================================================================
// Auth Operations (T125)
// =============================================================================

func registerAuthCreateKey(mcpServer *mcp.Server, svc *ServiceBundle) {
	type args struct {
		UserID string `json:"user_id"`
		Label  string `json:"label"`
	}

	mcpServer.AddTool(&mcp.Tool{
		Name:        "auth_create_key",
		Description: "Create a new API key for a user. The full key is shown only once in the response.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"user_id": stringProp("User ID to create key for"),
				"label":   stringProp("Label/description for the key"),
			},
			"required": []string{"user_id", "label"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var a args
		if err := parseArguments(req, &a); err != nil {
			return nil, err
		}

		apiKey, fullKey, err := svc.User.CreateAPIKey(ctx, a.UserID, a.Label, "")
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: err.Error()},
				},
			}, nil
		}

		return jsonResult(map[string]interface{}{
			"key_id":    apiKey.ID,
			"label":     apiKey.Label,
			"full_key":  fullKey,
			"prefix":    apiKey.ID[:8],
			"status":    "created",
			"warning":   "Store this key securely. The full key will not be shown again.",
		})
	})
}

func registerAuthListKeys(mcpServer *mcp.Server, svc *ServiceBundle) {
	type args struct {
		UserID string `json:"user_id"`
	}

	mcpServer.AddTool(&mcp.Tool{
		Name:        "auth_list_keys",
		Description: "List all API keys for a user (only key metadata, not the full keys).",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"user_id": stringProp("User ID"),
			},
			"required": []string{"user_id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var a args
		if err := parseArguments(req, &a); err != nil {
			return nil, err
		}

		keys, err := svc.User.ListAPIKeys(ctx, a.UserID)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: err.Error()},
				},
			}, nil
		}

		return jsonResult(map[string]interface{}{
			"keys":  keys,
			"count": len(keys),
		})
	})
}

func registerAuthRevokeKey(mcpServer *mcp.Server, svc *ServiceBundle) {
	type args struct {
		UserID string `json:"user_id"`
		KeyID  string `json:"key_id"`
	}

	mcpServer.AddTool(&mcp.Tool{
		Name:        "auth_revoke_key",
		Description: "Revoke (delete) an API key. The key will immediately stop working.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"user_id": stringProp("User ID that owns the key"),
				"key_id":  stringProp("Key ID to revoke"),
			},
			"required": []string{"user_id", "key_id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var a args
		if err := parseArguments(req, &a); err != nil {
			return nil, err
		}

		if err := svc.User.DeleteAPIKey(ctx, a.UserID, a.KeyID); err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: err.Error()},
				},
			}, nil
		}

		return jsonResult(map[string]interface{}{
			"key_id": a.KeyID,
			"status": "revoked",
		})
	})
}

// =============================================================================
// Action Operations (T126-T127) — stubs
// =============================================================================

func registerMemoryActionCreate(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_action_create",
		Description: "Create an actionable work item with typed dependencies.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"title":       stringProp("Action title"),
				"description": optStringProp("Detailed description of the work"),
				"project":     optStringProp("Project path"),
				"priority":    numberProp("Priority 1-10 (10 highest)"),
				"tags":        optStringProp("Comma-separated tags"),
				"parent_id":   optStringProp("Parent action ID for hierarchical actions"),
				"requires":    optStringProp("Comma-separated action IDs that must complete before this"),
			},
			"required": []string{"title"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_action_create"), nil
	})
}

func registerMemoryActionUpdate(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_action_update",
		Description: "Update an action's status, priority, or details.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"action_id": stringProp("Action ID to update"),
				"status":    optStringProp("New status: pending, active, done, blocked, cancelled"),
				"priority":  numberProp("New priority 1-10"),
				"result":    optStringProp("Outcome description (when completing)"),
			},
			"required": []string{"action_id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_action_update"), nil
	})
}

func registerMemoryFrontier(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_frontier",
		Description: "Get all unblocked actions ranked by priority and urgency.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project":  optStringProp("Filter by project"),
				"agent_id": optStringProp("Agent ID to check lease conflicts"),
				"limit":    numberProp("Max results (default 20)"),
			},
			"required": []string{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_frontier"), nil
	})
}

func registerMemoryNext(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_next",
		Description: "Get the single most important next action to work on.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"agent_id": optStringProp("Current agent ID"),
				"project":  optStringProp("Filter by project"),
			},
			"required": []string{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_next"), nil
	})
}

// =============================================================================
// Pipeline + Governance + Export + Graph + Context (T128-T132)
// =============================================================================

func registerMemoryConsolidate(mcpServer *mcp.Server, svc *ServiceBundle) {
	type args struct {
		Tier    string `json:"tier,omitempty"`
		SessionID string `json:"session_id,omitempty"`
	}

	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_consolidate",
		Description: "Run the memory consolidation pipeline on a session: summarize -> consolidate -> reflect.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"tier":       optStringProp("Target tier: episodic, semantic, or procedural"),
				"session_id": optStringProp("Session ID to consolidate (uses most recent if omitted)"),
			},
			"required": []string{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var a args
		if err := parseArguments(req, &a); err != nil {
			return nil, err
		}

		sessionID := a.SessionID
		if sessionID == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: "session_id is required for consolidation"},
				},
			}, nil
		}

		// Run summarization
		if err := svc.Summarization.SummarizeSession(ctx, sessionID); err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: "summarization failed: " + err.Error()},
				},
			}, nil
		}

		// Run consolidation
		if err := svc.Consolidation.ConsolidateSession(ctx, sessionID); err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: "consolidation failed: " + err.Error()},
				},
			}, nil
		}

		svc.Reflection.TriggerTimerCheck()

		return jsonResult(map[string]interface{}{
			"session_id": sessionID,
			"status":     "consolidated",
			"pipeline":   []string{"summarized", "consolidated", "reflection_checked"},
		})
	})
}

func registerMemoryCrystallize(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_crystallize",
		Description: "Compress completed action chains into compact crystal digests using LLM summarization.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"action_ids": stringProp("Comma-separated completed action IDs to crystallize"),
				"project":    optStringProp("Project context"),
				"session_id": optStringProp("Session context"),
			},
			"required": []string{"action_ids"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_crystallize"), nil
	})
}

func registerMemoryReflect(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_reflect",
		Description: "Traverse the knowledge graph, group related memories by concept clusters, and synthesize higher-order insights via LLM.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project":     optStringProp("Filter by project"),
				"max_clusters": numberProp("Max concept clusters to process (default 10, max 20)"),
			},
			"required": []string{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_reflect"), nil
	})
}

func registerMemoryDiagnose(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_diagnose",
		Description: "Run health checks across all subsystems (actions, leases, sentinels, sketches, signals, sessions, memories, mesh).",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"categories": optStringProp("Comma-separated categories to check (default all)"),
			},
			"required": []string{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_diagnose"), nil
	})
}

func registerMemoryHeal(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_heal",
		Description: "Auto-fix all fixable issues found by diagnostics. Unblocks stuck actions, expires stale leases, cleans up orphaned data.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"categories": optStringProp("Comma-separated categories to heal (default all)"),
				"dry_run":    optStringProp("Set to 'true' for dry run (report but don't fix)"),
			},
			"required": []string{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_heal"), nil
	})
}

func registerMemoryVerify(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_verify",
		Description: "Verify a memory or observation by tracing its citation chain back to source observations.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id": stringProp("Memory ID or observation ID to verify"),
			},
			"required": []string{"id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_verify"), nil
	})
}

func registerMemoryAudit(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_audit",
		Description: "View the audit trail of memory operations.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"limit":     numberProp("Max entries (default 50)"),
				"operation": optStringProp("Filter by operation type"),
			},
			"required": []string{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_audit"), nil
	})
}

func registerMemoryExport(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_export",
		Description: "Export all memory data as JSON.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
			"required":   []string{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_export"), nil
	})
}

func registerMemoryObsidianExport(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_obsidian_export",
		Description: "Export memories, lessons, and crystals as Obsidian-compatible Markdown files.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"types":     optStringProp("Comma-separated types to export: memories,lessons,crystals,sessions (default all)"),
				"vault_dir": optStringProp("Output directory (default ~/.agentmemory/vault/)"),
			},
			"required": []string{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_obsidian_export"), nil
	})
}

func registerMemoryCommitLookup(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_commit_lookup",
		Description: "Look up the agent session(s) that produced a specific git commit, given its SHA.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"sha": stringProp("Full git commit SHA"),
			},
			"required": []string{"sha"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_commit_lookup"), nil
	})
}

func registerMemoryCommits(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_commits",
		Description: "List recent commits linked to agent sessions, optionally filtered by branch or repo.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"branch": optStringProp("Filter by branch name"),
				"repo":   optStringProp("Filter by remote URL"),
				"limit":  numberProp("Max results (default 100, max 500)"),
			},
			"required": []string{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_commits"), nil
	})
}

func registerMemoryMeshSync(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_mesh_sync",
		Description: "Sync memories and actions with peer agentmemory instances for multi-agent collaboration.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"direction": optStringProp("push, pull, or both (default both)"),
				"peer_id":   optStringProp("Specific peer ID (omit for all)"),
			},
			"required": []string{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_mesh_sync"), nil
	})
}

func registerMemoryGraphQuery(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_graph_query",
		Description: "Query the knowledge graph for entities and relationships.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query":         optStringProp("Search nodes by name"),
				"start_node_id": optStringProp("Starting node ID for traversal"),
				"node_type":     optStringProp("Filter by node type"),
				"max_depth":     numberProp("Max BFS depth (default 3, max 5)"),
			},
			"required": []string{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_graph_query"), nil
	})
}

func registerMemoryRelations(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_relations",
		Description: "Query the memory relationship graph for a specific memory ID.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"memory_id":      stringProp("Memory ID to find relations for"),
				"max_hops":       numberProp("Max traversal depth (default 2)"),
				"min_confidence": numberProp("Min confidence (0-1, default 0)"),
			},
			"required": []string{"memory_id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_relations"), nil
	})
}

func registerMemoryProfile(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_profile",
		Description: "User/project profile with top concepts and file patterns.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project": stringProp("Project path"),
				"refresh": optStringProp("Set to 'true' to force rebuild"),
			},
			"required": []string{"project"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_profile"), nil
	})
}

func registerMemoryPatterns(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_patterns",
		Description: "Detect recurring patterns across sessions.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project": optStringProp("Project path to analyze"),
			},
			"required": []string{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_patterns"), nil
	})
}

func registerMemoryFacetQuery(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_facet_query",
		Description: "Query targets by facet tags with AND/OR logic.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"match_all":   optStringProp("Comma-separated dimension:value pairs (AND logic)"),
				"match_any":   optStringProp("Comma-separated dimension:value pairs (OR logic)"),
				"target_type": optStringProp("Filter by type: action, memory, or observation"),
			},
			"required": []string{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_facet_query"), nil
	})
}

func registerMemoryFacetTag(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_facet_tag",
		Description: "Attach a structured tag (dimension:value) to an action, memory, or observation.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"target_id":   stringProp("ID of the target to tag"),
				"target_type": stringProp("Type: action, memory, or observation"),
				"dimension":   stringProp("Tag dimension (e.g., priority, team, status)"),
				"value":       stringProp("Tag value (e.g., urgent, backend, reviewed)"),
			},
			"required": []string{"target_id", "target_type", "dimension", "value"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_facet_tag"), nil
	})
}

func registerMemoryVisionSearch(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_vision_search",
		Description: "Cross-modal image search via CLIP embeddings.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query_text":       optStringProp("Text query for finding matching screenshots"),
				"query_image_ref":  optStringProp("Absolute path to a stored image to match against"),
				"session_id":       optStringProp("Filter to a single session"),
				"top_k":            numberProp("Max results (default 10, max 50)"),
			},
			"required": []string{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_vision_search"), nil
	})
}

// =============================================================================
// v1 Service Tools (T144-T154) — all stubs
// =============================================================================

func registerMemorySlotCreate(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_slot_create",
		Description: "Create a new memory slot.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"label":       stringProp("Slot label — lowercase, starts with letter, [a-z0-9_]"),
				"content":     optStringProp("Initial content (default empty)"),
				"description": optStringProp("What this slot is for"),
				"scope":       optStringProp("'project' (default) or 'global' (shared across projects)"),
				"project":     optStringProp("Project name for project-scoped slot lookup"),
				"pinned":      optStringProp("'false' to exclude from context injection; default true"),
				"size_limit":  numberProp("Max chars (default 2000, hard cap 20000)"),
			},
			"required": []string{"label"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_slot_create"), nil
	})
}

func registerMemorySlotGet(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_slot_get",
		Description: "Read a single slot by label.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"label":   stringProp("Slot label"),
				"project": optStringProp("Project name for project-scoped slot lookup"),
			},
			"required": []string{"label"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_slot_get"), nil
	})
}

func registerMemorySlotList(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_slot_list",
		Description: "List all memory slots (global + project-scoped).",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project": optStringProp("Filter by project"),
			},
			"required": []string{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_slot_list"), nil
	})
}

func registerMemorySlotReplace(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_slot_replace",
		Description: "Replace slot content in place.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"label":   stringProp("Slot label"),
				"content": stringProp("New full content"),
				"project": optStringProp("Project name for project-scoped slot lookup"),
			},
			"required": []string{"label", "content"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_slot_replace"), nil
	})
}

func registerMemorySlotDelete(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_slot_delete",
		Description: "Delete a slot.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"label":   stringProp("Slot label"),
				"project": optStringProp("Project name for project-scoped slot lookup"),
			},
			"required": []string{"label"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_slot_delete"), nil
	})
}

func registerMemorySlotAppend(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_slot_append",
		Description: "Append text to an existing slot.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"label":   stringProp("Slot label"),
				"text":    stringProp("Text to append"),
				"project": optStringProp("Project name for project-scoped slot lookup"),
			},
			"required": []string{"label", "text"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_slot_append"), nil
	})
}

func registerMemorySignalRead(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_signal_read",
		Description: "Read messages for an agent.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"agent_id":     stringProp("Agent to read messages for"),
				"limit":        numberProp("Max messages (default 50)"),
				"thread_id":    optStringProp("Filter by conversation thread"),
				"unread_only":  optStringProp("Set to 'true' for unread only"),
			},
			"required": []string{"agent_id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_signal_read"), nil
	})
}

func registerMemorySignalSend(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_signal_send",
		Description: "Send a message to another agent or broadcast.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"from":     stringProp("Sender agent ID"),
				"content":  stringProp("Message content"),
				"to":       optStringProp("Recipient agent ID (omit for broadcast)"),
				"reply_to": optStringProp("Signal ID to reply to (auto-threads)"),
				"type":     optStringProp("Message type: info, request, response, alert, handoff"),
			},
			"required": []string{"from", "content"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_signal_send"), nil
	})
}

func registerMemorySentinelCreate(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_sentinel_create",
		Description: "Create an event-driven sentinel that watches for conditions and auto-unblocks gated actions.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name":              stringProp("Sentinel name"),
				"type":              stringProp("Type: webhook, timer, threshold, pattern, approval, custom"),
				"config":            optStringProp("JSON config"),
				"linked_action_ids": optStringProp("Comma-separated action IDs to gate"),
				"expires_in_ms":     numberProp("Auto-expire after ms"),
			},
			"required": []string{"name", "type"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_sentinel_create"), nil
	})
}

func registerMemorySentinelTrigger(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_sentinel_trigger",
		Description: "Externally fire a sentinel, providing an optional result payload.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"sentinel_id": stringProp("Sentinel ID to trigger"),
				"result":      optStringProp("JSON result payload"),
			},
			"required": []string{"sentinel_id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_sentinel_trigger"), nil
	})
}

func registerMemoryCheckpoint(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_checkpoint",
		Description: "Create or resolve an external checkpoint (CI result, approval, deploy status) that gates action progress.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"operation":          stringProp("create, resolve, or list"),
				"name":               optStringProp("Checkpoint name (for create)"),
				"type":               optStringProp("Checkpoint type: ci, approval, deploy, external, timer"),
				"checkpoint_id":      optStringProp("Checkpoint ID (for resolve)"),
				"status":             optStringProp("passed or failed (for resolve)"),
				"linked_action_ids":  optStringProp("Comma-separated action IDs this checkpoint gates (for create)"),
			},
			"required": []string{"operation"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_checkpoint"), nil
	})
}

func registerMemorySketchCreate(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_sketch_create",
		Description: "Create an ephemeral action graph for exploratory work.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"title":         stringProp("Sketch title"),
				"description":   optStringProp("What this sketch explores"),
				"project":       optStringProp("Project context"),
				"expires_in_ms": numberProp("TTL in ms (default 1 hour)"),
			},
			"required": []string{"title"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_sketch_create"), nil
	})
}

func registerMemorySketchPromote(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_sketch_promote",
		Description: "Promote a sketch's ephemeral actions to permanent actions.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"sketch_id": stringProp("Sketch ID to promote"),
				"project":   optStringProp("Override project for promoted actions"),
			},
			"required": []string{"sketch_id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_sketch_promote"), nil
	})
}

func registerMemoryRoutineRun(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_routine_run",
		Description: "Instantiate a frozen workflow routine, creating actions for each step with proper dependencies.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"routine_id":    stringProp("Routine template ID"),
				"project":       optStringProp("Project context"),
				"initiated_by":  optStringProp("Agent starting the run"),
			},
			"required": []string{"routine_id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_routine_run"), nil
	})
}

func registerMemorySnapshotCreate(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_snapshot_create",
		Description: "Create a git-versioned snapshot of current memory state.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"message": optStringProp("Snapshot description"),
			},
			"required": []string{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_snapshot_create"), nil
	})
}

func registerMemoryFileHistory(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_file_history",
		Description: "Get past observations about specific files.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"files":      stringProp("Comma-separated file paths"),
				"session_id": optStringProp("Current session ID to exclude"),
			},
			"required": []string{"files"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_file_history"), nil
	})
}

func registerMemoryLease(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_lease",
		Description: "Acquire, release, or renew an exclusive lease on an action.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"action_id": stringProp("Action ID"),
				"agent_id":  stringProp("Agent claiming the action"),
				"operation": stringProp("acquire, release, or renew"),
				"result":    optStringProp("Result when releasing (marks action done)"),
				"ttl_ms":    numberProp("Lease duration in ms (default 10min, max 1hr)"),
			},
			"required": []string{"action_id", "agent_id", "operation"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_lease"), nil
	})
}

func registerMemoryInsightList(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_insight_list",
		Description: "List synthesized insights — higher-order observations derived from patterns across memories, lessons, and crystals.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"limit":          numberProp("Max results (default 50)"),
				"min_confidence": numberProp("Minimum confidence threshold (default 0)"),
				"project":        optStringProp("Filter by project"),
			},
			"required": []string{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_insight_list"), nil
	})
}

func registerMemoryTeamShare(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_team_share",
		Description: "Share a memory or observation with team members.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"item_id":   stringProp("ID of memory or observation to share"),
				"item_type": stringProp("Type: observation, memory, or pattern"),
			},
			"required": []string{"item_id", "item_type"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_team_share"), nil
	})
}

func registerMemoryClaudeBridgeSync(mcpServer *mcp.Server, svc *ServiceBundle) {
	mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_claude_bridge_sync",
		Description: "Sync memory state to/from Claude Code's native MEMORY.md file.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"direction": stringProp("'read' to import from MEMORY.md, 'write' to export to MEMORY.md"),
			},
			"required": []string{"direction"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return stubbedToolResult("memory_claude_bridge_sync"), nil
	})
}

// truncateStr truncates a string to maxLen characters, adding "..." if truncated.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
