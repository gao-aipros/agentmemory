package service

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms"
)

// TestBuildGraphExtractionPrompt verifies the prompt structure includes
// observation metadata, enforces token budgets, and contains JSON format instructions.
func TestBuildGraphExtractionPrompt(t *testing.T) {
	t.Run("includes observation title and narrative", func(t *testing.T) {
		obs := []ObservationForExtraction{
			{
				Title:     "Refactoring auth module",
				Narrative: "The user refactored the authentication module to use JWT tokens instead of session cookies.",
				Concepts:  []string{"authentication", "JWT", "security"},
				Files:     []string{"auth.ts", "jwt.ts"},
			},
		}
		prompt := buildGraphExtractionPrompt(obs)

		assert.Contains(t, prompt, "Refactoring auth module", "prompt should contain observation title")
		assert.Contains(t, prompt, "The user refactored the authentication module to use JWT tokens instead of session cookies.", "prompt should contain narrative")
		assert.Contains(t, prompt, "authentication", "prompt should contain concept")
		assert.Contains(t, prompt, "JWT", "prompt should contain concept")
		assert.Contains(t, prompt, "security", "prompt should contain concept")
		assert.Contains(t, prompt, "auth.ts", "prompt should contain file reference")
		assert.Contains(t, prompt, "jwt.ts", "prompt should contain file reference")
	})

	t.Run("contains JSON output format instruction", func(t *testing.T) {
		obs := []ObservationForExtraction{
			{Title: "Test", Narrative: "Content"},
		}
		prompt := buildGraphExtractionPrompt(obs)

		assert.Contains(t, prompt, "JSON", "prompt should mention JSON format")
		assert.Contains(t, prompt, "entities", "prompt should reference entities array")
		assert.Contains(t, prompt, "relationships", "prompt should reference relationships array")
	})

	t.Run("truncates observation texts when exceeding token budget", func(t *testing.T) {
		longNarrative := strings.Repeat("This is a very long observation text that should be truncated. ", 200)
		obs := []ObservationForExtraction{
			{
				Title:     "Long observation",
				Narrative: longNarrative,
				Concepts:  []string{"testing", "truncation"},
				Files:     []string{"large_file.ts"},
			},
		}
		prompt := buildGraphExtractionPrompt(obs)

		// The full long narrative should not appear verbatim — truncation must occur
		assert.NotContains(t, prompt, longNarrative, "prompt should truncate observation text exceeding token budget")
	})
}

// TestParseExtractionResponse tests parsing of the LLM JSON response
// into entities and relationships.
func TestParseExtractionResponse(t *testing.T) {
	t.Run("valid JSON with entities and relationships", func(t *testing.T) {
		response := `{
			"entities": [
				{"type": "file", "name": "auth.ts", "properties": {"language": "typescript"}},
				{"type": "file", "name": "jwt.ts", "properties": {}}
			],
			"relationships": [
				{"type": "uses", "source": "auth.ts", "target": "jwt.ts", "weight": 0.9}
			]
		}`
		entities, relationships, err := ParseExtractionResponse(response)
		require.NoError(t, err)
		require.Len(t, entities, 2)
		require.Len(t, relationships, 1)

		assert.Equal(t, "file", entities[0].Type)
		assert.Equal(t, "auth.ts", entities[0].Name)
		assert.Equal(t, "typescript", entities[0].Properties["language"])

		assert.Equal(t, "file", entities[1].Type)
		assert.Equal(t, "jwt.ts", entities[1].Name)

		assert.Equal(t, "uses", relationships[0].Type)
		assert.Equal(t, "auth.ts", relationships[0].Source)
		assert.Equal(t, "jwt.ts", relationships[0].Target)
		assert.InDelta(t, 0.9, relationships[0].Weight, 1e-6)
	})

	t.Run("empty JSON object produces zero results", func(t *testing.T) {
		entities, relationships, err := ParseExtractionResponse(`{}`)
		require.NoError(t, err)
		assert.Empty(t, entities)
		assert.Empty(t, relationships)
	})

	t.Run("empty entities and relationships arrays produce zero results", func(t *testing.T) {
		response := `{"entities": [], "relationships": []}`
		entities, relationships, err := ParseExtractionResponse(response)
		require.NoError(t, err)
		assert.Empty(t, entities)
		assert.Empty(t, relationships)
	})

	t.Run("malformed JSON returns error", func(t *testing.T) {
		_, _, err := ParseExtractionResponse(`{entities: [}`)
		require.Error(t, err)
	})
}

// TestEntityIDComputation verifies that computeEntityID produces
// stable, unique, hex-encoded identifiers from (label, node_type) pairs.
func TestEntityIDComputation(t *testing.T) {
	t.Run("same inputs produce identical ID", func(t *testing.T) {
		id1 := computeEntityID("auth.ts", "file")
		id2 := computeEntityID("auth.ts", "file")
		assert.Equal(t, id1, id2, "same (label, node_type) should produce identical ID")
	})

	t.Run("different inputs produce different IDs", func(t *testing.T) {
		id1 := computeEntityID("auth.ts", "file")
		id2 := computeEntityID("jwt.ts", "file")
		id3 := computeEntityID("auth.ts", "concept")
		assert.NotEqual(t, id1, id2, "different labels should produce different IDs")
		assert.NotEqual(t, id1, id3, "different node types should produce different IDs")
		assert.NotEqual(t, id2, id3, "fully different inputs should produce different IDs")
	})

	t.Run("output is hex-encoded from sha256 first 16 bytes", func(t *testing.T) {
		id := computeEntityID("auth.ts", "file")
		// sha256 first 16 bytes hex-encoded = 32 hex characters
		assert.Len(t, id, 32, "hex encoding of sha256 first 16 bytes should produce 32 characters")
		_, err := hex.DecodeString(id)
		assert.NoError(t, err, "ID should be a valid hex-encoded string")
	})

	t.Run("empty inputs are handled deterministically", func(t *testing.T) {
		id1 := computeEntityID("", "")
		id2 := computeEntityID("", "")
		assert.Equal(t, id1, id2, "empty inputs should produce deterministic ID")
		assert.NotEmpty(t, id1, "empty inputs should still produce a non-empty ID")
	})
}

// TestEdgeTypePrefix verifies that prefixEdgeType correctly applies
// the "llm:" prefix to edge types without double-prefixing.
func TestEdgeTypePrefix(t *testing.T) {
	t.Run("adds llm prefix to clean edge type", func(t *testing.T) {
		result := prefixEdgeType("uses")
		assert.Equal(t, "llm:uses", result)
	})

	t.Run("does not double prefix already prefixed edge type", func(t *testing.T) {
		result := prefixEdgeType("llm:uses")
		assert.Equal(t, "llm:uses", result, "already-prefixed types should not be double-prefixed")
	})

	t.Run("handles empty edge type gracefully", func(t *testing.T) {
		result := prefixEdgeType("")
		// An empty string edge type may still receive the prefix
		assert.Equal(t, "llm:", result, "empty edge type should still receive prefix")
	})

	t.Run("does not double prefix when prefix is interior", func(t *testing.T) {
		// Only prefix when "llm:" is at the very start;
		// "something:llm:uses" has "llm:" in the interior, so it still gets prefixed.
		result := prefixEdgeType("something:llm:uses")
		assert.Equal(t, "llm:something:llm:uses", result)
	})
}

// TestStripMarkdownFences verifies that stripMarkdownFences correctly
// removes markdown code fences that some LLMs wrap around JSON output.
func TestStripMarkdownFences(t *testing.T) {
	t.Run("plain JSON unchanged", func(t *testing.T) {
		input := `{"entities":[],"relationships":[]}`
		result := stripMarkdownFences(input)
		assert.Equal(t, input, result)
	})
	t.Run("json code fence stripped", func(t *testing.T) {
		result := stripMarkdownFences("```json\n{\"entities\":[],\"relationships\":[]}\n```")
		assert.Equal(t, `{"entities":[],"relationships":[]}`, result)
	})
	t.Run("generic code fence stripped", func(t *testing.T) {
		result := stripMarkdownFences("```\n{\"entities\":[],\"relationships\":[]}\n```")
		assert.Equal(t, `{"entities":[],"relationships":[]}`, result)
	})
	t.Run("whitespace tolerant", func(t *testing.T) {
		result := stripMarkdownFences("  ```json\n{\"x\":1}\n```  ")
		assert.Equal(t, `{"x":1}`, result)
	})
}

// TestDedupStringSlice verifies that dedupStringSlice removes duplicates
// while preserving order.
func TestDedupStringSlice(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		result := dedupStringSlice(nil)
		assert.Nil(t, result)
	})
	t.Run("no duplicates", func(t *testing.T) {
		result := dedupStringSlice([]string{"a", "b", "c"})
		assert.Equal(t, []string{"a", "b", "c"}, result)
	})
	t.Run("duplicates removed", func(t *testing.T) {
		result := dedupStringSlice([]string{"a", "b", "a", "c", "b"})
		assert.Equal(t, []string{"a", "b", "c"}, result)
	})
}

// ---------------------------------------------------------------------------
// mockExtractionModel implements llms.Model for testing graph extraction.
type mockExtractionModel struct {
	callFunc func(ctx context.Context, prompt string, options ...llms.CallOption) (string, error)
}

func (m *mockExtractionModel) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	return m.callFunc(ctx, prompt, options...)
}

func (m *mockExtractionModel) GenerateContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	return nil, nil
}

// mockGraphExtractionQuerier implements graphExtractionQuerier for testing.
// It stores nodes and edges in-memory, simulating upsert-on-conflict behavior
// so that dedup can be verified.
type mockGraphExtractionQuerier struct {
	mu            sync.Mutex
	compressedObs map[string][]store.CompressedObservation
	nodes         map[string]store.GraphNode
	edges         map[string]store.GraphEdge
}

func (m *mockGraphExtractionQuerier) ListCompressedBySession(ctx context.Context, sessionID string) ([]store.CompressedObservation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.compressedObs[sessionID], nil
}

func (m *mockGraphExtractionQuerier) UpsertGraphNode(ctx context.Context, params store.UpsertGraphNodeParams) (store.GraphNode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Simulate ON CONFLICT (label, node_type) DO UPDATE
	for _, existing := range m.nodes {
		if existing.Label == params.Label && existing.NodeType == params.NodeType {
			existing.SourceObsIds = append(existing.SourceObsIds, params.SourceObsIds...)
			existing.Metadata = params.Metadata
			m.nodes[existing.ID] = existing
			return existing, nil
		}
	}

	node := store.GraphNode{
		ID:           params.ID,
		NodeType:     params.NodeType,
		EntityID:     params.EntityID,
		Label:        params.Label,
		Metadata:     params.Metadata,
		SourceObsIds: params.SourceObsIds,
	}
	m.nodes[params.ID] = node
	return node, nil
}

func (m *mockGraphExtractionQuerier) UpsertGraphEdge(ctx context.Context, params store.UpsertGraphEdgeParams) (store.GraphEdge, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Simulate ON CONFLICT (from_node_id, to_node_id, edge_type) DO UPDATE
	for _, existing := range m.edges {
		if existing.FromNodeID == params.FromNodeID && existing.ToNodeID == params.ToNodeID && existing.EdgeType == params.EdgeType {
			existing.Weight = (existing.Weight + params.Weight) / 2.0
			existing.SourceObsIds = append(existing.SourceObsIds, params.SourceObsIds...)
			m.edges[existing.ID] = existing
			return existing, nil
		}
	}

	edge := store.GraphEdge{
		ID:           params.ID,
		FromNodeID:   params.FromNodeID,
		ToNodeID:     params.ToNodeID,
		EdgeType:     params.EdgeType,
		Weight:       params.Weight,
		SourceObsIds: params.SourceObsIds,
	}
	m.edges[params.ID] = edge
	return edge, nil
}

// TestExtractFromSession verifies the full ExtractFromSession flow:
// listing compressed observations, building a prompt, calling the (mock) LLM,
// parsing the response, and upserting graph nodes/edges with the expected data.
func TestExtractFromSession(t *testing.T) {
	mockQ := &mockGraphExtractionQuerier{
		compressedObs: map[string][]store.CompressedObservation{
			"session-1": {
				{
					ID:             "comp-1",
					ObservationIds: []string{"obs-1", "obs-2"},
					SessionID:      "session-1",
					CompressedText: "Refactored auth module to use JWT tokens instead of session cookies.",
					Concepts:       []string{"auth", "JWT", "security"},
				},
			},
		},
		nodes: make(map[string]store.GraphNode),
		edges: make(map[string]store.GraphEdge),
	}

	mockL := &mockExtractionModel{
		callFunc: func(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
			return `{
				"entities": [
					{"type": "file", "name": "auth.ts", "properties": {"language": "typescript"}},
					{"type": "file", "name": "jwt.ts", "properties": {}}
				],
				"relationships": [
					{"type": "uses", "source": "auth.ts", "target": "jwt.ts", "weight": 0.9}
				]
			}`, nil
		},
	}

	llmSvc := NewLLMServiceWithModel(mockL)
	svc := newGraphExtractionServiceWithQuerier(mockQ, llmSvc)

	// First extraction: verify entities and relationships are created.
	entities, edges, err := svc.ExtractFromSession(context.Background(), "session-1")
	require.NoError(t, err)
	assert.Equal(t, 2, entities, "should extract 2 entities")
	assert.Equal(t, 1, edges, "should extract 1 relationship")

	// Verify graph_nodes were upserted with correct data.
	mockQ.mu.Lock()
	assert.Len(t, mockQ.nodes, 2, "should have 2 graph nodes")

	var nodeLabels []string
	for _, n := range mockQ.nodes {
		nodeLabels = append(nodeLabels, n.Label)
		assert.Equal(t, "obs-1", n.EntityID, "entity_id should be the first observation ID")
		assert.Equal(t, "file", n.NodeType, "node type should be 'file'")
	}
	assert.Contains(t, nodeLabels, "auth.ts")
	assert.Contains(t, nodeLabels, "jwt.ts")

	// Verify graph_edges were upserted with llm: prefix.
	assert.Len(t, mockQ.edges, 1, "should have 1 graph edge")
	for _, e := range mockQ.edges {
		assert.True(t, strings.HasPrefix(e.EdgeType, "llm:"), "edge type should have llm: prefix")
		assert.Equal(t, "llm:uses", e.EdgeType)
		assert.InDelta(t, 0.9, e.Weight, 1e-6)

		// Verify edge links to existing nodes.
		_, srcOK := mockQ.nodes[e.FromNodeID]
		_, dstOK := mockQ.nodes[e.ToNodeID]
		assert.True(t, srcOK, "edge source should be a known node")
		assert.True(t, dstOK, "edge target should be a known node")
	}
	mockQ.mu.Unlock()

	// Second extraction with same data: verify dedup (no new nodes/edges).
	_, _, err = svc.ExtractFromSession(context.Background(), "session-1")
	require.NoError(t, err)

	mockQ.mu.Lock()
	assert.Len(t, mockQ.nodes, 2, "should not create duplicate nodes after second extraction")
	assert.Len(t, mockQ.edges, 1, "should not create duplicate edges after second extraction")
	mockQ.mu.Unlock()
}

// TestExtractFromSession_EmptySession verifies that ExtractFromSession
// handles a session with no compressed observations gracefully.
func TestExtractFromSession_EmptySession(t *testing.T) {
	mockQ := &mockGraphExtractionQuerier{
		compressedObs: map[string][]store.CompressedObservation{
			"empty-session": {},
		},
		nodes: make(map[string]store.GraphNode),
		edges: make(map[string]store.GraphEdge),
	}

	mockL := &mockExtractionModel{
		callFunc: func(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
			t.Error("LLM should not be called for empty session")
			return "", nil
		},
	}

	llmSvc := NewLLMServiceWithModel(mockL)
	svc := newGraphExtractionServiceWithQuerier(mockQ, llmSvc)

	entities, edges, err := svc.ExtractFromSession(context.Background(), "empty-session")
	require.NoError(t, err)
	assert.Equal(t, 0, entities, "no entities for empty session")
	assert.Equal(t, 0, edges, "no relationships for empty session")
}

// TestExtractFromSession_EmptyLLMResponse verifies that the service handles
// an LLM response with no entities or relationships gracefully.
func TestExtractFromSession_EmptyLLMResponse(t *testing.T) {
	mockQ := &mockGraphExtractionQuerier{
		compressedObs: map[string][]store.CompressedObservation{
			"session-2": {
				{
					ID:             "comp-2",
					ObservationIds: []string{"obs-3"},
					SessionID:      "session-2",
					CompressedText: "Some observation with no extractable entities.",
				},
			},
		},
		nodes: make(map[string]store.GraphNode),
		edges: make(map[string]store.GraphEdge),
	}

	mockL := &mockExtractionModel{
		callFunc: func(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
			return `{"entities": [], "relationships": []}`, nil
		},
	}

	llmSvc := NewLLMServiceWithModel(mockL)
	svc := newGraphExtractionServiceWithQuerier(mockQ, llmSvc)

	entities, edges, err := svc.ExtractFromSession(context.Background(), "session-2")
	require.NoError(t, err)
	assert.Equal(t, 0, entities, "no entities should be returned")
	assert.Equal(t, 0, edges, "no relationships should be returned")

	// Verify no nodes or edges were upserted.
	assert.Empty(t, mockQ.nodes, "no nodes should be upserted")
	assert.Empty(t, mockQ.edges, "no edges should be upserted")
}

// ---------------------------------------------------------------------------
// Phase 4: User Story 2 — Graph-Enriched Context

// TestGraphTraversal_SourceObsIDsOverlap verifies that the GraphTraversal CTE
// can discover entity B via the source_obs_ids overlap branch when seeded with
// observation IDs that match entity A's source_obs_ids.
// This is a conceptual integration test using mocked querier responses.
func TestGraphTraversal_SourceObsIDsOverlap(t *testing.T) {
	mockQ := &mockContextQuerier{
		listObservationsByUserID: func(ctx context.Context, params store.ListObservationsByUserIDParams) ([]store.Observation, error) {
			return []store.Observation{
				{ID: "obs-A1", Narrative: "Entity A related observation"},
				{ID: "obs-A2", Narrative: "Another Entity A observation"},
			}, nil
		},
		graphTraversal: func(ctx context.Context, params store.GraphTraversalParams) ([]store.GraphTraversalRow, error) {
			// Verify seed IDs include observations from entity A's source_obs_ids
			assert.Contains(t, params.Column1, "obs-A1", "seed IDs should include obs-A1 from source_obs_ids")
			assert.Contains(t, params.Column1, "obs-A2", "seed IDs should include obs-A2 from source_obs_ids")
			// Return entity B's observation as discovered via the source_obs_ids overlap branch
			return []store.GraphTraversalRow{
				{ID: "obs-B1", GraphScore: 0.85},
			}, nil
		},
		getObservationsByIDs: func(ctx context.Context, ids []string) ([]store.Observation, error) {
			return []store.Observation{
				{ID: "obs-B1", Narrative: "Entity B discovered via graph traversal overlap"},
			}, nil
		},
	}

	svc := &ContextService{queries: mockQ}
	result, err := svc.gatherGraphNeighbors(context.Background(), "user-1")
	require.NoError(t, err)
	assert.NotEmpty(t, result, "should discover entity B via source_obs_ids overlap")
	assert.Contains(t, result, "obs-B1", "output should contain entity B's observation ID")
	assert.Contains(t, result, "0.85", "output should contain the graph score from traversal")
}

// TestLLMExtractedEdges_Distinguishable verifies that LLM-extracted edges
// (with "llm:" prefix) are distinguishable from structural edges (bare type)
// in the stored data.
func TestLLMExtractedEdges_Distinguishable(t *testing.T) {
	// Pre-seed a structural edge with bare type (no llm: prefix).
	// This simulates edges inserted by non-LLM code paths.
	nodeAID := computeEntityID("entity-a", "file")
	nodeBID := computeEntityID("entity-b", "file")

	mockQ := &mockGraphExtractionQuerier{
		compressedObs: map[string][]store.CompressedObservation{
			"session-distinct": {
				{
					ID:             "comp-1",
					ObservationIds: []string{"obs-1"},
					SessionID:      "session-distinct",
					CompressedText: "Entity A uses Entity B.",
				},
			},
		},
		nodes: map[string]store.GraphNode{
			nodeAID: {ID: nodeAID, Label: "entity-a", NodeType: "file"},
			nodeBID: {ID: nodeBID, Label: "entity-b", NodeType: "file"},
		},
		edges: map[string]store.GraphEdge{
			"structural-edge-1": {
				ID:         "structural-edge-1",
				FromNodeID: nodeAID,
				ToNodeID:   nodeBID,
				EdgeType:   "imports", // bare type, no llm: prefix
				Weight:     0.5,
			},
		},
	}

	mockL := &mockExtractionModel{
		callFunc: func(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
			return `{
				"entities": [
					{"type": "file", "name": "entity-a", "properties": {}},
					{"type": "file", "name": "entity-b", "properties": {}}
				],
				"relationships": [
					{"type": "uses", "source": "entity-a", "target": "entity-b", "weight": 0.9}
				]
			}`, nil
		},
	}

	llmSvc := NewLLMServiceWithModel(mockL)
	svc := newGraphExtractionServiceWithQuerier(mockQ, llmSvc)

	_, _, err := svc.ExtractFromSession(context.Background(), "session-distinct")
	require.NoError(t, err)

	mockQ.mu.Lock()
	defer mockQ.mu.Unlock()

	// Verify both edges exist
	assert.Len(t, mockQ.edges, 2, "should have both structural and LLM-extracted edges")

	// Categorize edges by prefix
	var structuralEdges, llmEdges int
	for _, e := range mockQ.edges {
		if strings.HasPrefix(e.EdgeType, "llm:") {
			llmEdges++
			assert.Equal(t, "llm:uses", e.EdgeType, "LLM edge should have llm:uses type")
		} else {
			structuralEdges++
			assert.Equal(t, "imports", e.EdgeType, "structural edge should have bare 'imports' type")
		}
	}
	assert.Equal(t, 1, structuralEdges, "should have exactly 1 structural edge")
	assert.Equal(t, 1, llmEdges, "should have exactly 1 LLM-extracted edge")
}

// ---------------------------------------------------------------------------
// Phase 5: User Story 3 — Graceful Degradation

// TestExtractFromSession_LLMError verifies that when the LLM call fails,
// ExtractFromSession returns (0, 0, error) without panicking.
func TestExtractFromSession_LLMError(t *testing.T) {
	mockQ := &mockGraphExtractionQuerier{
		compressedObs: map[string][]store.CompressedObservation{
			"session-llm-err": {
				{
					ID:             "comp-1",
					ObservationIds: []string{"obs-1"},
					SessionID:      "session-llm-err",
					CompressedText: "Some observation text.",
				},
			},
		},
		nodes: make(map[string]store.GraphNode),
		edges: make(map[string]store.GraphEdge),
	}

	// Mock LLM that returns an error
	mockL := &mockExtractionModel{
		callFunc: func(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
			return "", fmt.Errorf("LLM API error: rate limited")
		},
	}

	llmSvc := NewLLMServiceWithModel(mockL)
	svc := newGraphExtractionServiceWithQuerier(mockQ, llmSvc)

	entities, edges, err := svc.ExtractFromSession(context.Background(), "session-llm-err")
	assert.Error(t, err, "should return error when LLM call fails")
	assert.Contains(t, err.Error(), "LLM call failed", "error should wrap the LLM failure")
	assert.Equal(t, 0, entities, "should return 0 entities on LLM error")
	assert.Equal(t, 0, edges, "should return 0 edges on LLM error")

	// Verify no partial data was persisted
	mockQ.mu.Lock()
	assert.Empty(t, mockQ.nodes, "no nodes should be created when LLM fails")
	assert.Empty(t, mockQ.edges, "no edges should be created when LLM fails")
	mockQ.mu.Unlock()
}

// TestParseExtractionResponse_MalformedJSON verifies additional edge cases
// for malformed JSON beyond the basic case tested in T008.
func TestParseExtractionResponse_MalformedJSON(t *testing.T) {
	t.Run("missing closing brace", func(t *testing.T) {
		_, _, err := ParseExtractionResponse(`{"entities": [{"type": "file", "name": "test.ts"}]`)
		require.Error(t, err, "missing closing brace should produce error")
	})

	t.Run("invalid types in field", func(t *testing.T) {
		// type field expects a string, but gets a number
		_, _, err := ParseExtractionResponse(`{"entities": [{"type": 123, "name": "test.ts"}], "relationships": []}`)
		require.Error(t, err, "wrong JSON type for entity type should produce error")
	})

	t.Run("null entities array", func(t *testing.T) {
		entities, relationships, err := ParseExtractionResponse(`{"entities": null, "relationships": null}`)
		require.NoError(t, err, "null arrays should not produce error")
		assert.Empty(t, entities, "null entities should produce empty slice")
		assert.Empty(t, relationships, "null relationships should produce empty slice")
	})

	t.Run("null entity name", func(t *testing.T) {
		// null name becomes empty string and the entity is skipped
		entities, relationships, err := ParseExtractionResponse(`{"entities": [{"type": "file", "name": null}], "relationships": []}`)
		require.NoError(t, err, "null name should not produce error")
		assert.Empty(t, entities, "entity with null name should be skipped")
		assert.Empty(t, relationships, "no relationships should be returned")
	})

	t.Run("trailing comma", func(t *testing.T) {
		_, _, err := ParseExtractionResponse(`{"entities": [], "relationships": [],}`)
		require.Error(t, err, "trailing comma should produce error")
	})

	t.Run("unexpected token", func(t *testing.T) {
		_, _, err := ParseExtractionResponse(`{invalid}`)
		require.Error(t, err, "unexpected token should produce error")
	})
}

// TestExtractFromSession_ConflictingRelationships verifies that when two
// relationships with different types exist between the same entities, both
// are stored as separate rows (different edge_type = different row per ON CONFLICT).
func TestExtractFromSession_ConflictingRelationships(t *testing.T) {
	mockQ := &mockGraphExtractionQuerier{
		compressedObs: map[string][]store.CompressedObservation{
			"session-conflict": {
				{
					ID:             "comp-1",
					ObservationIds: []string{"obs-1", "obs-2"},
					SessionID:      "session-conflict",
					CompressedText: "Entity A uses and depends on Entity B.",
				},
			},
		},
		nodes: make(map[string]store.GraphNode),
		edges: make(map[string]store.GraphEdge),
	}

	// LLM returns conflicting relationship types between same entities
	mockL := &mockExtractionModel{
		callFunc: func(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
			return `{
				"entities": [
					{"type": "file", "name": "entity-a", "properties": {}},
					{"type": "file", "name": "entity-b", "properties": {}}
				],
				"relationships": [
					{"type": "uses", "source": "entity-a", "target": "entity-b", "weight": 0.9},
					{"type": "depends_on", "source": "entity-a", "target": "entity-b", "weight": 0.7}
				]
			}`, nil
		},
	}

	llmSvc := NewLLMServiceWithModel(mockL)
	svc := newGraphExtractionServiceWithQuerier(mockQ, llmSvc)

	entities, edges, err := svc.ExtractFromSession(context.Background(), "session-conflict")
	require.NoError(t, err)
	assert.Equal(t, 2, entities, "should extract 2 entities")
	assert.Equal(t, 2, edges, "should extract 2 conflicting relationships")

	mockQ.mu.Lock()
	defer mockQ.mu.Unlock()

	// Both edges should exist with different types
	assert.Len(t, mockQ.edges, 2, "both edges should be stored (different edge_type = different row)")

	edgeTypes := make(map[string]bool)
	for _, e := range mockQ.edges {
		edgeTypes[e.EdgeType] = true
	}
	assert.True(t, edgeTypes["llm:uses"], "llm:uses edge should be stored")
	assert.True(t, edgeTypes["llm:depends_on"], "llm:depends_on edge should be stored")
	assert.Len(t, edgeTypes, 2, "should have exactly 2 distinct edge types")
}

// BenchmarkExtractionBatch10 benchmarks ExtractFromSession with a batch of 10
// compressed observations. Target: completion within 60 seconds (SC-005).
// With mocks this should be near-instant.
func BenchmarkExtractionBatch10(b *testing.B) {
	// Create 10 compressed observations with varied content.
	compressedObs := make([]store.CompressedObservation, 10)
	for i := 0; i < 10; i++ {
		compressedObs[i] = store.CompressedObservation{
			ID:             fmt.Sprintf("comp-%d", i),
			ObservationIds: []string{fmt.Sprintf("obs-%d", i)},
			SessionID:      "bench-session",
			CompressedText: fmt.Sprintf("Observation %d: The user worked on authentication module refactoring to improve security and performance.", i),
			Concepts:       []string{"auth", "JWT", "security", "refactoring"},
		}
	}

	mockL := &mockExtractionModel{
		callFunc: func(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
			return `{
				"entities": [
					{"type": "concept", "name": "JWT", "properties": {}},
					{"type": "concept", "name": "authentication", "properties": {}},
					{"type": "concept", "name": "security", "properties": {}}
				],
				"relationships": [
					{"type": "depends_on", "source": "JWT", "target": "authentication", "weight": 0.9},
					{"type": "improves", "source": "JWT", "target": "security", "weight": 0.7}
				]
			}`, nil
		},
	}

	llmSvc := NewLLMServiceWithModel(mockL)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Fresh mock querier each iteration so every benchmark run starts clean.
		mockQ := &mockGraphExtractionQuerier{
			compressedObs: map[string][]store.CompressedObservation{
				"bench-session": compressedObs,
			},
			nodes: make(map[string]store.GraphNode),
			edges: make(map[string]store.GraphEdge),
		}
		svc := newGraphExtractionServiceWithQuerier(mockQ, llmSvc)

		_, _, err := svc.ExtractFromSession(context.Background(), "bench-session")
		if err != nil {
			b.Fatal(err)
		}
	}
}
