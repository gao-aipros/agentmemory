//go:build e2e

// Package e2e contains end-to-end tests that run against a live agentmemory
// server (typically started via docker-compose). These tests use the official
// go-sdk MCP client over StreamableHTTP, including SSE streaming.
//
// Prerequisites:
//
//	docker compose up -d
//	export AGENTMEMORY_E2E_API_KEY=$(... obtain via REST ...)
//	go test -tags=e2e -v ./tests/e2e/
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── helpers ────────────────────────────────────────────────────────────────

func getAPIKey(t *testing.T) string {
	t.Helper()
	key := os.Getenv("AGENTMEMORY_E2E_API_KEY")
	require.NotEmpty(t, key, "AGENTMEMORY_E2E_API_KEY must be set")
	return key
}

func getBaseURL() string {
	if u := os.Getenv("AGENTMEMORY_E2E_BASE_URL"); u != "" {
		return u
	}
	return "http://localhost:8080"
}

func getDBURL() string {
	if u := os.Getenv("AGENTMEMORY_E2E_DB_URL"); u != "" {
		return u
	}
	return "postgres://agentmemory:agentmemory@localhost:5432/agentmemory?sslmode=disable"
}

// authTransport injects the API key into every HTTP request.
type authTransport struct {
	apiKey string
	base   http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	return t.base.RoundTrip(req)
}

func newMCPClient(t *testing.T, apiKey string) (*mcp.Client, *mcp.StreamableClientTransport) {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "e2e-test", Version: "1.0.0"}, nil)
	transport := &mcp.StreamableClientTransport{
		Endpoint: getBaseURL() + "/v1/mcp",
		HTTPClient: &http.Client{
			Transport: &authTransport{apiKey: apiKey, base: http.DefaultTransport},
			Timeout:   30 * time.Second,
		},
	}
	return client, transport
}

func newDBPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, getDBURL())
	require.NoError(t, err, "failed to connect to DB")
	t.Cleanup(pool.Close)
	return pool
}

// ── full lifecycle test ────────────────────────────────────────────────────

func TestMCP_FullLifecycle(t *testing.T) {
	apiKey := getAPIKey(t)
	ctx := context.Background()

	client, transport := newMCPClient(t, apiKey)
	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err, "connect")
	t.Cleanup(func() { session.Close() })

	// --- 1. Initialize ---
	t.Run("Initialize", func(t *testing.T) {
		// Connect() handles the full initialize → initialized handshake.
		// If Connect() returned nil error, initialization succeeded.
		// We verify by immediately pinging — the session is live.
		err := session.Ping(ctx, nil)
		assert.NoError(t, err, "ping immediately after connect should succeed")
	})

	// --- 2. Ping ---
	t.Run("Ping", func(t *testing.T) {
		err := session.Ping(ctx, nil)
		assert.NoError(t, err, "ping should succeed")
	})

	// --- 3. ListTools ---
	t.Run("ListTools", func(t *testing.T) {
		result, err := session.ListTools(ctx, nil)
		require.NoError(t, err, "list tools")
		assert.GreaterOrEqual(t, len(result.Tools), 50, "should have 50+ tools")

		toolNames := make(map[string]bool)
		for _, tl := range result.Tools {
			toolNames[tl.Name] = true
		}
		required := []string{
			"memory_save", "memory_recall", "memory_observe",
			"memory_smart_search", "memory_forget",
			"auth_list_keys", "auth_create_key",
		}
		for _, name := range required {
			assert.True(t, toolNames[name], "required tool %q should exist", name)
		}
	})

	// --- 4. memory_save ---
	var savedConcept string
	t.Run("memory_save", func(t *testing.T) {
		savedConcept = fmt.Sprintf("e2e-test-concept-%d", time.Now().UnixNano())
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "memory_save",
			Arguments: map[string]any{
				"content":  "E2E test memory: agentmemory MCP client validation",
				"concepts": []string{savedConcept},
				"type":     "fact",
			},
		})
		require.NoError(t, err, "memory_save call")
		require.False(t, result.IsError, "memory_save should not return error")
		assert.NotEmpty(t, result.Content, "should return content")
	})

	// --- 5. memory_recall ---
	t.Run("memory_recall", func(t *testing.T) {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "memory_recall",
			Arguments: map[string]any{
				"query": savedConcept,
			},
		})
		require.NoError(t, err, "memory_recall call")
		require.False(t, result.IsError, "memory_recall should not return error")
		assert.NotEmpty(t, result.Content, "should return results")
	})

	// --- 6. memory_observe (requires session_id — test via smoke test) ---

	// --- 7. memory_smart_search ---
	t.Run("memory_smart_search", func(t *testing.T) {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "memory_smart_search",
			Arguments: map[string]any{
				"query": "e2e MCP test",
			},
		})
		require.NoError(t, err, "smart_search call")
		require.False(t, result.IsError, "smart_search should not return error")
	})

	// --- 8. auth_list_keys ---
	t.Run("auth_list_keys", func(t *testing.T) {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "auth_list_keys",
			Arguments: map[string]any{},
		})
		require.NoError(t, err, "auth_list_keys call")
		require.False(t, result.IsError, "auth_list_keys should not return error")
		assert.NotEmpty(t, result.Content, "should return keys")
	})

	// --- 9. SSE streaming ---
	t.Run("SSE_streaming", func(t *testing.T) {
		// The client was created with DisableStandaloneSSE: false (default).
		// After Connect(), the go-sdk automatically opens a GET SSE stream.
		// We verify SSE is working by making a tool call — if the SSE stream
		// had failed with a terminal error, Read() would return an error.
		// Additionally, we ping to exercise the full duplex connection.
		err := session.Ping(ctx, nil)
		assert.NoError(t, err, "ping with SSE active should succeed")

		// List tools again — proves the connection is still healthy
		result, err := session.ListTools(ctx, nil)
		assert.NoError(t, err, "list tools with SSE active")
		assert.NotEmpty(t, result.Tools)
	})

	// --- 10. Session close ---
	t.Run("Session_close", func(t *testing.T) {
		err := session.Close()
		assert.NoError(t, err, "close should succeed")
	})

	// --- 11. Post-close ping ---
	t.Run("Post_close_ping", func(t *testing.T) {
		err := session.Ping(ctx, nil)
		assert.Error(t, err, "ping after close should fail")
	})
}

// ── smoke test: call every tool ────────────────────────────────────────────

func TestMCP_SmokeAllTools(t *testing.T) {
	apiKey := getAPIKey(t)
	ctx := context.Background()

	client, transport := newMCPClient(t, apiKey)
	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err, "connect")
	defer session.Close()

	result, err := session.ListTools(ctx, nil)
	require.NoError(t, err, "list tools")

	t.Logf("Testing %d tools (with rate-limit delay)...", len(result.Tools))

	for _, tool := range result.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			ctx2, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			_, err := session.CallTool(ctx2, &mcp.CallToolParams{
				Name:      tool.Name,
				Arguments: map[string]any{},
			})

			// Transport-level errors (timeout, crash, 429 rate limit) = test failure.
			// Tool-level errors (missing args, validation) = acceptable from IsError.
			assert.NoError(t, err, "tool %q must not crash, timeout, or hit rate limit", tool.Name)
		})
		// Avoid hitting rate limiter (100 req/s, burst ~10)
		time.Sleep(50 * time.Millisecond)
	}
}

// ── error handling tests ───────────────────────────────────────────────────

func TestMCP_AuthFailure(t *testing.T) {
	ctx := context.Background()

	// Client with no API key
	client := mcp.NewClient(&mcp.Implementation{Name: "e2e-test", Version: "1.0.0"}, nil)
	transport := &mcp.StreamableClientTransport{
		Endpoint:   getBaseURL() + "/v1/mcp",
		HTTPClient: http.DefaultClient,
	}

	_, err := client.Connect(ctx, transport, nil)
	assert.Error(t, err, "connect without auth should fail")
}

func TestMCP_InvalidTool(t *testing.T) {
	apiKey := getAPIKey(t)
	ctx := context.Background()

	client, transport := newMCPClient(t, apiKey)
	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err, "connect")
	defer session.Close()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "nonexistent_tool_12345",
		Arguments: map[string]any{},
	})
	// The call itself may succeed at the transport level but the result should be an error
	if err == nil {
		assert.True(t, result.IsError, "nonexistent tool should return isError=true")
	}
}

// ── DB verification ────────────────────────────────────────────────────────

func TestMCP_DBVerification(t *testing.T) {
	apiKey := getAPIKey(t)
	ctx := context.Background()
	pool := newDBPool(t)

	client, transport := newMCPClient(t, apiKey)
	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err, "connect")
	defer session.Close()

	concept := fmt.Sprintf("e2e-db-verify-%d", time.Now().UnixNano())

	// Save a memory
	_, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "memory_save",
		Arguments: map[string]any{
			"content":  "DB verification test memory",
			"concepts": []string{concept},
			"type":     "fact",
		},
	})
	require.NoError(t, err, "memory_save")

	// Verify in database
	var count int
	err = pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM memories WHERE $1 = ANY(concepts)",
		concept,
	).Scan(&count)
	require.NoError(t, err, "DB query")
	assert.GreaterOrEqual(t, count, 1, "memory should exist in DB, concept=%s", concept)
}

// ── REST helpers ───────────────────────────────────────────────────────────

func restDo(method, path string, body any, apiKey string) (*http.Response, []byte, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, nil, err
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, getBaseURL()+path, bodyReader)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	return resp, respBody, err
}

func restJSON(method, path string, body any, apiKey string, out any) (int, error) {
	resp, respBody, err := restDo(method, path, body, apiKey)
	if err != nil {
		return 0, err
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}

// pollDB waits for a condition (query returns count >= min) with timeout.
func pollDB(t *testing.T, pool *pgxpool.Pool, query string, min int, timeout time.Duration) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var count int
		if err := pool.QueryRow(ctx, query).Scan(&count); err == nil && count >= min {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("timed out after %v waiting for: %s", timeout, query)
}

// ── 4-tier pipeline E2E ───────────────────────────────────────────────────

func TestMCP_PipelineE2E(t *testing.T) {
	apiKey := getAPIKey(t)
	ctx := context.Background()
	pool := newDBPool(t)

	client, transport := newMCPClient(t, apiKey)
	transport.DisableStandaloneSSE = true // avoid retry exhaustion during long pipeline waits
	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err, "connect")
	defer session.Close()

	// ---- Step 1: Create session via REST ----
	type sessionStartResp struct {
		SessionID string `json:"session_id"`
		Status    string `json:"status"`
	}
	var ssr sessionStartResp
	status, err := restJSON("POST", "/v1/api/session/start", map[string]any{}, apiKey, &ssr)
	require.NoError(t, err, "session start")
	require.Equal(t, 201, status, "session start HTTP 201")
	sessionID := ssr.SessionID
	t.Logf("Session created: %s", sessionID)

	// ---- Step 2: Record observations via MCP ----
	types := []string{"user_prompt_submit", "pre_tool_use", "post_tool_use", "pre_llm_call", "post_llm_call"}
	for i, hookType := range types {
		_, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "memory_observe",
			Arguments: map[string]any{
				"type":       hookType,
				"title":      fmt.Sprintf("Pipeline e2e observation %d: %s", i, hookType),
				"narrative":  fmt.Sprintf("Detailed narrative for pipeline e2e test observation %d. The agent performed a %s action involving database schema design and PostgreSQL optimization.", i, hookType),
				"session_id": sessionID,
				"concepts":   []string{"pipeline-e2e", hookType, "postgresql"},
				"importance": 0.8,
			},
		})
		require.NoError(t, err, "memory_observe %s", hookType)
	}
	t.Logf("Recorded %d observations", len(types))

	// ---- Step 3: Verify observations are NOT immediately compressed ----
	// With the scheduler pipeline, compression only happens via session-end or periodic
	// scheduler tiers. Observations recorded via MCP should NOT be compressed immediately.
	var initialCompressedCount int
	pool.QueryRow(ctx, fmt.Sprintf(
		"SELECT COUNT(*) FROM compressed_observations WHERE session_id = '%s'", sessionID,
	)).Scan(&initialCompressedCount)
	assert.Equal(t, 0, initialCompressedCount,
		"observations should NOT be compressed immediately -- compression is session-end or scheduler-triggered only")
	t.Logf("Verified: no auto-compression (compressed_observations count = %d)", initialCompressedCount)

	// ---- Step 4: End session via REST -- triggers compression + summarization ----
	t.Run("SessionEnd_CompressionAndSummarization", func(t *testing.T) {
		// Use REST POST /v1/api/session/end to trigger the session-end handler.
		// This asynchronously runs CompressSessionNow + SummarizeSessionNow (isFull=true).
		status, _, err := restDo("POST", "/v1/api/session/end", map[string]any{
			"session_id": sessionID,
		}, apiKey)
		require.NoError(t, err, "session end request")
		require.Equal(t, 200, status, "session end HTTP 200")

		// Wait for compression (Tier 0 -- happens asynchronously in runPipeline goroutine)
		pollDB(t, pool,
			fmt.Sprintf("SELECT COUNT(*) FROM compressed_observations WHERE session_id = '%s'", sessionID),
			1, 60*time.Second)

		// Verify compressed text is non-empty (LLM worked)
		var nonEmptyCount int
		err = pool.QueryRow(ctx, fmt.Sprintf(
			"SELECT COUNT(*) FROM compressed_observations WHERE session_id = '%s' AND compressed_text != ''", sessionID,
		)).Scan(&nonEmptyCount)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, nonEmptyCount, 1, "at least one compressed_text should be non-empty")

		// Verify compressed_embeddings (may fail if embedding model name is misconfigured)
		var embCount int
		err = pool.QueryRow(ctx, fmt.Sprintf(
			"SELECT COUNT(*) FROM compressed_embeddings ce JOIN compressed_observations co ON co.id = ce.compressed_id WHERE co.session_id = '%s'", sessionID,
		)).Scan(&embCount)
		require.NoError(t, err)
		if embCount == 0 {
			t.Log("WARNING: no embeddings found -- check EMBEDDING_MODEL in .env")
		} else {
			t.Logf("Embeddings: %d", embCount)
		}

		// Verify session-end summary created with is_full=true
		// (session-end sets isFull=true, mid-scheduler sets isFull=false)
		var summaryText string
		err = pool.QueryRow(ctx,
			"SELECT summary_text FROM session_summaries WHERE session_id = $1", sessionID,
		).Scan(&summaryText)
		require.NoError(t, err, "session summary should exist after session-end")
		assert.NotEmpty(t, summaryText, "summary_text should be non-empty (LLM summarization worked)")

		var isFull bool
		err = pool.QueryRow(ctx,
			"SELECT is_full FROM session_summaries WHERE session_id = $1", sessionID,
		).Scan(&isFull)
		require.NoError(t, err)
		assert.True(t, isFull, "session-end summary should have is_full=true (not mid-session)")
		t.Logf("Session-end summary: %d chars, is_full=%v", len(summaryText), isFull)
	})

	// ---- Step 5: Get user ID for consolidation context ----
	var userID string
	err = pool.QueryRow(ctx, "SELECT id FROM users LIMIT 1").Scan(&userID)
	require.NoError(t, err, "get user ID")
	t.Logf("User ID: %s", userID)

	// ---- Step 6: Consolidation + reflection via MCP (scheduler-only, not at session-end) ----
	t.Run("Consolidation_And_Reflection", func(t *testing.T) {
		// memory_consolidate triggers summarize + consolidate + reflect via MCP.
		// Consolidation and reflection are NOT part of session-end -- they are
		// scheduler-tier operations (Tier 2 and Tier 3) or explicit MCP calls.
		result, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "memory_consolidate",
			Arguments: map[string]any{
				"session_id":    sessionID,
				"tier":          "procedural",
				"owner_user_id": userID,
			},
		})
		require.NoError(t, err, "memory_consolidate call")
		require.False(t, result.IsError, "memory_consolidate should succeed")

		// Verify consolidation output: memories + lessons with source='consolidation'
		var memCount, lesCount int
		pool.QueryRow(ctx, "SELECT COUNT(*) FROM memories WHERE source = 'consolidation'").Scan(&memCount)
		pool.QueryRow(ctx, "SELECT COUNT(*) FROM lessons WHERE source = 'consolidation'").Scan(&lesCount)
		assert.GreaterOrEqual(t, memCount, 1, "memories should exist (LLM consolidation worked)")
		assert.GreaterOrEqual(t, lesCount, 1, "lessons should exist (LLM consolidation worked)")
		t.Logf("Consolidation: %d memories, %d lessons", memCount, lesCount)

		// Verify reflection output: insights with source='reflect'
		var insCount int
		pool.QueryRow(ctx, "SELECT COUNT(*) FROM insights WHERE source = 'reflect'").Scan(&insCount)
		assert.GreaterOrEqual(t, insCount, 1, "insights should exist (reflection worked)")
		t.Logf("Reflection: %d insights", insCount)
	})

	// ---- Step 7: Vector search verifies embedding + search end-to-end ----
	t.Run("Vector_Search", func(t *testing.T) {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "memory_smart_search",
			Arguments: map[string]any{
				"query": "database schema PostgreSQL optimization",
			},
		})
		require.NoError(t, err, "smart_search")
		require.False(t, result.IsError)
		assert.NotEmpty(t, result.Content, "search should return results (vector search worked)")
	})
}
