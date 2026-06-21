package integration

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_FullAgentSession runs a complete end-to-end agent session:
// 1. Create user
// 2. Start session
// 3. Record observations across all 13 hook types
// 4. Search verifies observations are findable
// 5. Context injection assembles context
// 6. Full pipeline verified

func TestE2E_FullAgentSession(t *testing.T) {
	// Skip if Docker/testcontainers is unavailable
	db, err := setupTestDBSkipable(t)
	if err != nil {
		t.Skip("database unavailable — skipping e2e test: " + err.Error())
	}
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	// =========================================================================
	// Step 1: Create a user
	// =========================================================================
	userID := uuid.New().String()
	_, err = db.Pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash, name) VALUES ($1, $2, $3, $4)`,
		userID, "e2e-agent@example.com", "hash_value", "E2E Agent",
	)
	require.NoError(t, err, "should create e2e test user")

	// =========================================================================
	// Step 2: Start a session
	// =========================================================================
	sessionID := uuid.New().String()
	_, err = db.Pool.Exec(ctx,
		`INSERT INTO sessions (id, user_id, status) VALUES ($1, $2, 'active')`,
		sessionID, userID,
	)
	require.NoError(t, err, "should create test session")

	// =========================================================================
	// Step 3: Fire all 13 hooks in order with realistic observation data
	// =========================================================================
	obsSvc := service.NewObservationService(db.Pool, nil)

	allTypes := service.ValidHookTypes
	assert.Len(t, allTypes, 13, "should be exactly 13 valid hook types")

	// Realistic observation data for each hook type in a typical agent session
	type hookData struct {
		title      string
		narrative  string
		facts      string
		concepts   []string
		files      []string
		importance float64
	}

	observations := map[string]hookData{
		service.HookSessionStart: {
			title:     "Session started for bugfix task",
			narrative: "Agent session initiated to fix PostgreSQL connection timeout bug. Working directory: /home/user/project. Model: claude-sonnet-4-20250514. Max turns: 50.",
			facts:     `{"model": "claude-sonnet-4-20250514", "max_turns": 50, "working_dir": "/home/user/project", "api_key_source": "env"}`,
			concepts:  []string{"session", "agent", "initialization"},
			files:     []string{},
			importance: 0.9,
		},
		service.HookUserPromptSubmit: {
			title:     "User submitted bugfix prompt",
			narrative: "User asked: 'Fix the connection timeout in the PostgreSQL pool configuration. Connections are timing out after 5 seconds but our queries sometimes take up to 10 seconds.'",
			facts:     `{"prompt_length": 156, "attachments": 0, "has_context": false}`,
			concepts:  []string{"user_input", "bugfix", "postgresql", "timeout"},
			files:     []string{},
			importance: 0.8,
		},
		service.HookPreToolUse: {
			title:     "About to read database configuration",
			narrative: "Agent preparing to use Read tool to examine the PostgreSQL pool configuration file at internal/config/database.go.",
			facts:     `{"tool": "Read", "path": "internal/config/database.go", "tool_input_chars": 64}`,
			concepts:  []string{"tool_use", "read", "config", "database"},
			files:     []string{"internal/config/database.go"},
			importance: 0.6,
		},
		service.HookPostToolUse: {
			title:     "Successfully read database configuration",
			narrative: "Read internal/config/database.go: found connection pool configured with connMaxLifetime=5s. The default connection timeout is set to 5 seconds in the pool config. The health check timeout is also 5s. Lines 45-52 show the timeout configuration.",
			facts:     `{"tool": "Read", "file": "internal/config/database.go", "lines_read": 120, "tool_output_chars": 2456}`,
			concepts:  []string{"tool_result", "database", "config", "timeout"},
			files:     []string{"internal/config/database.go"},
			importance: 0.7,
		},
		service.HookPostToolUseFailure: {
			title:     "Failed to read missing file",
			narrative: "Agent attempted to Read internal/config/old_config.go but the file does not exist. This was a speculative read to find legacy timeout configurations.",
			facts:     `{"tool": "Read", "path": "internal/config/old_config.go", "error": "file not found", "error_type": "ENOENT"}`,
			concepts:  []string{"tool_failure", "read", "error", "filesystem"},
			files:     []string{"internal/config/old_config.go"},
			importance: 0.4,
		},
		service.HookPreCompact: {
			title:     "Context window approaching limit",
			narrative: "Token count reached 85% of context window (170000/200000 tokens). Triggering pre-compact hook to summarize conversation history before compaction occurs. Active observations: 8. Pending tool calls: 2.",
			facts:     `{"current_tokens": 170000, "max_tokens": 200000, "percentage": 85, "active_observations": 8, "pending_tool_calls": 2}`,
			concepts:  []string{"compaction", "context_window", "tokens", "memory_management"},
			files:     []string{},
			importance: 0.7,
		},
		service.HookSubagentStart: {
			title:     "Delegate search to subagent",
			narrative: "Spawning researcher subagent to search for PostgreSQL connection pool best practices across the codebase and online documentation. Subagent type: general-purpose. Task: find connection timeout recommendations.",
			facts:     `{"subagent_type": "general-purpose", "subagent_name": "researcher", "task": "search_codebase", "timeout_ms": 120000}`,
			concepts:  []string{"subagent", "delegation", "search", "research"},
			files:     []string{},
			importance: 0.6,
		},
		service.HookSubagentStop: {
			title:     "Subagent research completed",
			narrative: "Researcher subagent completed its task. Found 3 relevant files: internal/config/database.go, docs/CONNECTION_POOLING.md, and an old migration setting timeout to 30s. Agent recommended increasing timeout to 30s.",
			facts:     `{"subagent_name": "researcher", "files_found": 3, "duration_ms": 8500, "status": "completed", "recommendation": "increase_timeout_to_30s"}`,
			concepts:  []string{"subagent", "research", "postgresql", "timeout", "complete"},
			files:     []string{"internal/config/database.go", "docs/CONNECTION_POOLING.md"},
			importance: 0.7,
		},
		service.HookNotification: {
			title:     "Build verification notification",
			narrative: "Build system reported: go build ./... completed successfully after the timeout fix was applied. All 34 packages compiled without errors. No regressions detected.",
			facts:     `{"type": "build_success", "packages": 34, "errors": 0, "warnings": 0, "duration_ms": 4500}`,
			concepts:  []string{"build", "verification", "compilation", "success"},
			files:     []string{},
			importance: 0.5,
		},
		service.HookTaskCompleted: {
			title:     "Bugfix task completed",
			narrative: "Successfully fixed PostgreSQL connection timeout bug. Changed connMaxLifetime from 5s to 30s in internal/config/database.go. All tests pass. The connection pool now handles queries up to 30 seconds without timeout.",
			facts:     `{"task_type": "bugfix", "files_changed": 1, "lines_added": 2, "lines_removed": 2, "tests_passed": 12}`,
			concepts:  []string{"task_complete", "bugfix", "postgresql", "timeout", "success"},
			files:     []string{"internal/config/database.go"},
			importance: 0.9,
		},
		service.HookPostCommit: {
			title:     "Changes committed to repository",
			narrative: "Committed the timeout fix to branch fix/pg-timeout. Commit message: 'Increase connection pool timeout from 5s to 30s to handle long-running queries'.",
			facts:     `{"sha": "a1b2c3d4e5f6", "branch": "fix/pg-timeout", "files_changed": 1, "additions": 2, "deletions": 2}`,
			concepts:  []string{"git", "commit", "version_control", "bugfix"},
			files:     []string{"internal/config/database.go"},
			importance: 0.8,
		},
		service.HookSessionEnd: {
			title:     "Agent session ending",
			narrative: "Session completed successfully. Total duration: 12 minutes 34 seconds. Total observations: 13. Files modified: 1. All tasks completed. No unresolved items.",
			facts:     `{"duration_seconds": 754, "total_observations": 13, "files_modified": 1, "tasks_completed": 1, "unresolved_items": 0}`,
			concepts:  []string{"session", "completion", "summary", "success"},
			files:     []string{},
			importance: 0.9,
		},
		service.HookPermissionPrompt: {
			title:     "Agent requested write permission",
			narrative: "Agent requested permission to edit internal/config/database.go. The change modifies connection pool timeout from 5s to 30s. Permission was granted by user.",
			facts:     `{"tool": "Edit", "file": "internal/config/database.go", "permission_requested": "write", "permission_granted": true, "prompt_type": "tool_permission"}`,
			concepts:  []string{"permissions", "tool_use", "edit", "write"},
			files:     []string{"internal/config/database.go"},
			importance: 0.7,
		},
	}

	// Record all 13 hook observations in order
	recordedIDs := make([]string, 0, 13)
	for _, hookType := range allTypes {
		data, ok := observations[hookType]
		require.True(t, ok, "should have observation data for hook type: %s", hookType)

		obs, err := obsSvc.RecordObservation(ctx, service.RecordObservationInput{
			SessionID:   sessionID,
			OwnerUserID: userID,
			Type:        hookType,
			Title:       data.title,
			Narrative:   data.narrative,
			Facts:       data.facts,
			Concepts:    data.concepts,
			Files:       data.files,
			Importance:  ptrFloat64(data.importance),
		})
		require.NoError(t, err, "should record observation for hook: %s", hookType)
		require.NotNil(t, obs)
		assert.Equal(t, hookType, obs.Type, "observation type should match hook type")
		assert.NotEmpty(t, obs.ID, "observation should have an ID")
		recordedIDs = append(recordedIDs, obs.ID)
	}

	assert.Len(t, recordedIDs, 13, "should have recorded all 13 hook observations")

	// =========================================================================
	// Step 4: Run search to verify observations are findable
	// =========================================================================
	searchSvc := service.NewSearchService(db.Pool, nil)

	// Search for the PostgreSQL timeout content
	results, err := searchSvc.HybridSearch(ctx, "PostgreSQL connection timeout bugfix", 10)
	require.NoError(t, err, "search should succeed")
	require.NotEmpty(t, results, "search should return results for the bugfix content")

	// Verify at least one observation appears in search results
	foundIDs := make(map[string]bool)
	for _, r := range results {
		foundIDs[r.ID] = true
	}

	matchCount := 0
	for _, id := range recordedIDs {
		if foundIDs[id] {
			matchCount++
		}
	}
	assert.GreaterOrEqual(t, matchCount, 1,
		"at least one recorded observation should appear in search results")
	t.Logf("Search found %d/13 recorded observations", matchCount)

	// Search for permission-related content
	permResults, err := searchSvc.HybridSearch(ctx, "permission request write tool", 10)
	require.NoError(t, err)
	assert.NotEmpty(t, permResults, "should find permission-related observations")

	// =========================================================================
	// Step 5: Run context injection to verify it assembles context
	// =========================================================================
	slotSvc := service.NewSlotService(db.Pool)
	embedSvc := service.NewEmbeddingService(db.Pool, nil)
	ctxSvc := service.NewContextService(db.Pool, embedSvc, slotSvc)

	// Assemble context for the test user
	assembled, err := ctxSvc.AssembleContext(ctx, userID)
	require.NoError(t, err, "context assembly should succeed")
	require.NotNil(t, assembled)

	// Apply budget and format
	budget := service.DefaultContextBudget()
	formatted := service.ApplyBudget(assembled, budget)
	require.NotEmpty(t, formatted, "formatted context should not be empty")

	// Verify budget is respected
	tokens := service.EstimateTokens(formatted)
	assert.LessOrEqual(t, tokens, budget.TotalTokens,
		"assembled context must respect %d token budget (got %d tokens)",
		budget.TotalTokens, tokens)
	t.Logf("E2E context tokens: %d / %d", tokens, budget.TotalTokens)

	// Verify context contains expected elements
	assert.Contains(t, formatted, "Context (AgentMemory v2)",
		"context should contain agentmemory header")
	assert.Contains(t, formatted, "Date:",
		"context should contain date stamp")

	// Verify Observations bucket is populated
	assert.NotEmpty(t, assembled.Observations,
		"observations bucket should be populated from the recorded hooks")
	t.Logf("Observations bucket length: %d chars", len(assembled.Observations))

	// =========================================================================
	// Step 6: Verify full pipeline works end-to-end
	// =========================================================================

	// Verify end session — mark session as ended
	_, err = db.Pool.Exec(ctx,
		`UPDATE sessions SET status = 'ended', ended_at = now() WHERE id = $1`,
		sessionID,
	)
	require.NoError(t, err)

	// Verify the session status updated
	var status string
	err = db.Pool.QueryRow(ctx,
		`SELECT status FROM sessions WHERE id = $1`, sessionID,
	).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "ended", status)

	// Verify all 13 observations are still present after session end
	obsRows, err := db.Pool.Query(ctx,
		`SELECT COUNT(*) FROM observations WHERE session_id = $1`, sessionID,
	)
	require.NoError(t, err)
	defer obsRows.Close()

	var obsCount int
	if obsRows.Next() {
		err = obsRows.Scan(&obsCount)
		require.NoError(t, err)
	}
	assert.Equal(t, 13, obsCount,
		"all 13 observations should persist after session end")

	// Verify observation types are correct
	typeRows, err := db.Pool.Query(ctx,
		`SELECT type FROM observations WHERE session_id = $1 ORDER BY created_at`, sessionID,
	)
	require.NoError(t, err)
	defer typeRows.Close()

	var recordedTypes []string
	for typeRows.Next() {
		var obsType string
		err = typeRows.Scan(&obsType)
		require.NoError(t, err)
		recordedTypes = append(recordedTypes, obsType)
	}
	assert.Len(t, recordedTypes, 13, "should have 13 observation types")
	assert.Equal(t, allTypes, recordedTypes,
		"observation types should match the order of ValidHookTypes")

	t.Log("E2E pipeline verified: user creation -> session start -> 13 hooks -> search -> context injection -> session end")
}

// setupTestDBSkipable attempts to start a test database, returning an error
// instead of calling t.Fatal so the test can skip cleanly.
func setupTestDBSkipable(t *testing.T) (*TestDB, error) {
	t.Helper()

	// Check if Docker is likely available by looking for the docker socket
	if _, err := os.Stat("/var/run/docker.sock"); os.IsNotExist(err) {
		return nil, &testDBError{msg: "Docker socket not found"}
	}

	// Use a deferred recovery approach: call SetupTestDB and catch any fatals
	var testDB *TestDB
	var setupErr error

	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				setupErr = &testDBError{msg: fmt.Sprintf("testcontainer setup panicked: %v", r)}
			}
			close(done)
		}()
		testDB, setupErr = setupTestDBDirect(t)
	}()
	<-done

	if setupErr != nil {
		return nil, setupErr
	}
	return testDB, nil
}

type testDBError struct {
	msg string
}

func (e *testDBError) Error() string {
	return e.msg
}

// setupTestDBDirect creates a TestDB without using t.Fatalf, so callers can handle errors.
func setupTestDBDirect(t *testing.T) (*TestDB, error) {
	t.Helper()
	// Delegate to the standard SetupTestDB but catch any fatals
	var db *TestDB
	var err error

	func() {
		defer func() {
			if r := recover(); r != nil {
				db = nil
				err = &testDBError{msg: fmt.Sprintf("testcontainer setup panicked: %v", r)}
			}
		}()
		db = SetupTestDB(t)
	}()

	if err != nil {
		return nil, err
	}
	if db == nil {
		return nil, &testDBError{msg: "test database setup returned nil"}
	}
	return db, nil
}
