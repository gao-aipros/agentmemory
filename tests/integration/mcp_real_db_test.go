package integration

import (
	"encoding/json"
	"testing"

	"github.com/agentmemory/agentmemory/internal/store"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMCPRealDB_MemorySave tests that memory_save via MCP actually inserts a
// memory row into the real ParadeDB database.
func TestMCPRealDB_MemorySave(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	runMigrations(t, db)

	_, session, ctx, cancel := setupMCPServer(t, db.Pool)
	defer cancel()
	defer session.Close()

	// Call memory_save via MCP
	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "memory_save",
		Arguments: map[string]interface{}{
			"content":  "MCP real DB test — memory save",
			"type":     "fact",
			"project":  "test-project",
			"concepts": []string{"test", "mcp", "real-db"},
		},
	})
	require.NoError(t, err, "memory_save should not return protocol error")
	require.False(t, result.IsError, "memory_save should not set IsError=true")

	// Extract memory_id from the result content
	require.NotEmpty(t, result.Content, "memory_save result should have content")
	textContent, ok := result.Content[0].(*sdkmcp.TextContent)
	require.True(t, ok, "result content should be text")
	t.Logf("memory_save response: %s", textContent.Text)

	// Parse the JSON response to get memory_id
	var resp struct {
		MemoryID string `json:"memory_id"`
		Status   string `json:"status"`
	}
	require.NoError(t, json.Unmarshal([]byte(textContent.Text), &resp))
	require.NotEmpty(t, resp.MemoryID, "memory_id should not be empty")
	assert.Equal(t, "saved", resp.Status)

	// Verify the memory row exists in the database via direct ID lookup
	queries := store.New(db.Pool)
	retrieved, err := queries.GetMemory(ctx, resp.MemoryID)
	require.NoError(t, err, "should be able to retrieve the inserted memory by ID")
	assert.Equal(t, "MCP real DB test — memory save", retrieved.Content)
	t.Logf("Verified memory row: id=%s content=%s", retrieved.ID, retrieved.Content)
}

// TestMCPRealDB_LessonSave tests that memory_lesson_save via MCP actually inserts
// a lesson row into the real ParadeDB database.
func TestMCPRealDB_LessonSave(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	runMigrations(t, db)

	_, session, ctx, cancel := setupMCPServer(t, db.Pool)
	defer cancel()
	defer session.Close()

	// Call memory_lesson_save via MCP
	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "memory_lesson_save",
		Arguments: map[string]interface{}{
			"content":    "Always write tests before implementation code — it catches bugs early.",
			"context":    "TDD workflow",
			"project":    "test-project",
			"tags":       []string{"tdd", "testing", "best-practice"},
			"confidence": 0.9,
		},
	})
	require.NoError(t, err, "memory_lesson_save should not return protocol error")
	require.False(t, result.IsError, "memory_lesson_save should not set IsError=true")

	require.NotEmpty(t, result.Content, "memory_lesson_save result should have content")
	textContent, ok := result.Content[0].(*sdkmcp.TextContent)
	require.True(t, ok, "result content should be text")
	t.Logf("memory_lesson_save response: %s", textContent.Text)

	// Parse the JSON response to get lesson_id
	var resp struct {
		LessonID string `json:"lesson_id"`
		Status   string `json:"status"`
	}
	require.NoError(t, json.Unmarshal([]byte(textContent.Text), &resp))
	require.NotEmpty(t, resp.LessonID, "lesson_id should not be empty")
	assert.Equal(t, "saved", resp.Status)

	// Verify the lesson row exists in the database via direct ID lookup
	queries := store.New(db.Pool)
	retrieved, err := queries.GetLesson(ctx, resp.LessonID)
	require.NoError(t, err, "should be able to retrieve the inserted lesson by ID")
	assert.Equal(t, "Always write tests before implementation code — it catches bugs early.", retrieved.Content)
	assert.InDelta(t, 0.9, retrieved.Confidence, 0.01)
	t.Logf("Verified lesson row: id=%s content=%s confidence=%f", retrieved.ID, retrieved.Content, retrieved.Confidence)
}

// TestMCPRealDB_BothSaveAndRetrieve tests both memory_save and memory_lesson_save
// in a single test flow, then verifies both can be retrieved by ID from the DB.
func TestMCPRealDB_BothSaveAndRetrieve(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	runMigrations(t, db)

	_, session, ctx, cancel := setupMCPServer(t, db.Pool)
	defer cancel()
	defer session.Close()

	// Save a memory
	memResult, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "memory_save",
		Arguments: map[string]interface{}{
			"content": "Integration test memory — dual save test",
		},
	})
	require.NoError(t, err)
	require.False(t, memResult.IsError)

	memText, ok := memResult.Content[0].(*sdkmcp.TextContent)
	require.True(t, ok)
	var memResp struct {
		MemoryID string `json:"memory_id"`
		Status   string `json:"status"`
	}
	require.NoError(t, json.Unmarshal([]byte(memText.Text), &memResp))
	require.NotEmpty(t, memResp.MemoryID)

	// Save a lesson
	lessonResult, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "memory_lesson_save",
		Arguments: map[string]interface{}{
			"content":    "Integration test lesson — dual save test",
			"confidence": 0.75,
		},
	})
	require.NoError(t, err)
	require.False(t, lessonResult.IsError)

	lessonText, ok := lessonResult.Content[0].(*sdkmcp.TextContent)
	require.True(t, ok)
	var lessonResp struct {
		LessonID string `json:"lesson_id"`
		Status   string `json:"status"`
	}
	require.NoError(t, json.Unmarshal([]byte(lessonText.Text), &lessonResp))
	require.NotEmpty(t, lessonResp.LessonID)

	// Both should exist in the database via direct ID lookup
	queries := store.New(db.Pool)

	mem, err := queries.GetMemory(ctx, memResp.MemoryID)
	require.NoError(t, err, "should retrieve memory by ID")
	assert.Equal(t, "Integration test memory — dual save test", mem.Content)

	lesson, err := queries.GetLesson(ctx, lessonResp.LessonID)
	require.NoError(t, err, "should retrieve lesson by ID")
	assert.Equal(t, "Integration test lesson — dual save test", lesson.Content)
	assert.InDelta(t, 0.75, lesson.Confidence, 0.01)
}
