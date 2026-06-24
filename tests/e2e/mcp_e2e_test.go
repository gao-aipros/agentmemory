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
	"context"
	"fmt"
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
