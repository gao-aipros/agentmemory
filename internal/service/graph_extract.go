package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentmemory/agentmemory/internal/store"
)

// GRAPH_EXTRACTION_CHAR_BUDGET is the maximum total character length for
// observation narratives in a single graph extraction prompt (~750 tokens at
// 4 chars/token for English). This is a character budget, not a token budget.
const GRAPH_EXTRACTION_CHAR_BUDGET = 3000

// ObservationForExtraction holds the fields needed for graph extraction prompt building.
type ObservationForExtraction struct {
	Title     string
	Narrative string
	Concepts  []string
	Files     []string
}

// ParsedEntity represents a single entity extracted from an LLM response.
type ParsedEntity struct {
	Type       string
	Name       string
	Properties map[string]string
}

// ParsedRelationship represents a single relationship extracted from an LLM response.
type ParsedRelationship struct {
	Type   string
	Source string
	Target string
	Weight float64
}

// graphExtractionQuerier defines the database operations needed for graph extraction.
// *store.Queries satisfies this interface, enabling mock-based unit testing.
type graphExtractionQuerier interface {
	ListCompressedBySession(ctx context.Context, sessionID string) ([]store.CompressedObservation, error)
	UpsertGraphNode(ctx context.Context, params store.UpsertGraphNodeParams) (store.GraphNode, error)
	UpsertGraphEdge(ctx context.Context, params store.UpsertGraphEdgeParams) (store.GraphEdge, error)
}

// GraphExtractionService extracts knowledge graphs from compressed observations
// by sending them through an LLM and persisting the discovered entities and relationships.
type GraphExtractionService struct {
	queries graphExtractionQuerier
	llm     *LLMService
}

// NewGraphExtractionService creates a new GraphExtractionService backed by a
// real database pool and LLM service.
func NewGraphExtractionService(pool *pgxpool.Pool, llm *LLMService) *GraphExtractionService {
	return &GraphExtractionService{
		queries: store.New(pool),
		llm:     llm,
	}
}

// newGraphExtractionServiceWithQuerier creates a GraphExtractionService with a
// custom querier (for testing with mocks).
func newGraphExtractionServiceWithQuerier(q graphExtractionQuerier, llm *LLMService) *GraphExtractionService {
	return &GraphExtractionService{
		queries: q,
		llm:     llm,
	}
}

// entityInfo tracks resolved node metadata for edge creation.
type entityInfo struct {
	nodeType string
	nodeID   string
}

// ExtractFromSession extracts a knowledge graph from compressed observations
// for the given session. It returns the number of entities and relationships
// discovered, or an error if the LLM call or database operations fail.
func (s *GraphExtractionService) ExtractFromSession(ctx context.Context, sessionID string) (int, int, error) {
	// 1. List compressed observations for the session.
	compressedObs, err := s.queries.ListCompressedBySession(ctx, sessionID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to list compressed observations: %w", err)
	}
	if len(compressedObs) == 0 {
		return 0, 0, nil
	}

	// 2. Convert compressed observations to the extraction format.
	var allObsIDs []string
	obsForExtraction := make([]ObservationForExtraction, 0, len(compressedObs))
	for _, co := range compressedObs {
		title := co.CompressedText
		if len(title) > 80 {
			title = title[:80]
		}
		obsForExtraction = append(obsForExtraction, ObservationForExtraction{
			Title:     title,
			Narrative: co.CompressedText,
			Concepts:  co.Concepts,
			Files:     nil,
		})
		allObsIDs = append(allObsIDs, co.ObservationIds...)
	}
	// Deduplicate observation IDs to prevent array bloat from
	// repeated extraction of the same session.
	allObsIDs = dedupStringSlice(allObsIDs)

	// 3. Build the LLM prompt.
	prompt := buildGraphExtractionPrompt(obsForExtraction)

	// 4. Call the LLM.
	response, err := s.llm.Call(ctx, prompt)
	if err != nil {
		return 0, 0, fmt.Errorf("LLM call failed: %w", err)
	}

	// 5. Parse the LLM response. Strip markdown code fences first
	// because some LLMs wrap JSON in ```json ... ``` despite
	// instructions to output raw JSON only.
	entities, relationships, err := ParseExtractionResponse(stripMarkdownFences(response))
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse extraction response: %w", err)
	}
	if len(entities) == 0 && len(relationships) == 0 {
		return 0, 0, nil
	}

	// Determine the entity_id from the first observation ID.
	var entityID string
	if len(allObsIDs) > 0 {
		entityID = allObsIDs[0]
	}

	// Build a name -> (nodeType, nodeID) map from parsed entities.
	entityMap := make(map[string]entityInfo, len(entities))

	// 6. Upsert each entity as a graph node.
	for _, e := range entities {
		nodeID := computeEntityID(e.Name, e.Type)
		metadata, err := json.Marshal(e.Properties)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to marshal properties for entity %q: %w", e.Name, err)
		}

		if _, err := s.queries.UpsertGraphNode(ctx, store.UpsertGraphNodeParams{
			ID:           nodeID,
			NodeType:     e.Type,
			EntityID:     entityID,
			Label:        e.Name,
			Metadata:     metadata,
			SourceObsIds: allObsIDs,
		}); err != nil {
			return 0, 0, fmt.Errorf("failed to upsert graph node %q: %w", e.Name, err)
		}

		entityMap[e.Name] = entityInfo{nodeType: e.Type, nodeID: nodeID}
	}

	// 7. Upsert each relationship as a graph edge.
	for _, r := range relationships {
		src, srcOK := entityMap[r.Source]
		dst, dstOK := entityMap[r.Target]
		if !srcOK || !dstOK {
			continue
		}

		edgeType := prefixEdgeType(r.Type)

		if _, err := s.queries.UpsertGraphEdge(ctx, store.UpsertGraphEdgeParams{
			ID:           uuid.New().String(),
			FromNodeID:   src.nodeID,
			ToNodeID:     dst.nodeID,
			EdgeType:     edgeType,
			Weight:       r.Weight,
			SourceObsIds: allObsIDs,
		}); err != nil {
			return 0, 0, fmt.Errorf("failed to upsert graph edge: %w", err)
		}
	}

	return len(entities), len(relationships), nil
}

// buildGraphExtractionPrompt assembles observation data into an LLM prompt
// that requests entity and relationship extraction in JSON format.
func buildGraphExtractionPrompt(obs []ObservationForExtraction) string {
	var sb strings.Builder

	sb.WriteString(`You are a knowledge graph extraction engine. Given observation text from an agent session, discover entities and relationships.

Extract entities (files, functions, classes, concepts, errors, data types, modules, config items) and relationships between them.

Respond ONLY with a JSON object:
{
  "entities": [{"type": "file|function|class|concept|error|datatype|module|config", "name": "entity_name", "properties": {"key": "value"}}],
  "relationships": [{"type": "uses|imports|calls|depends_on|fixes|causes|contains|references", "source": "entity_name", "target": "entity_name", "weight": 0.0-1.0}]
}

Rules:
- entity.name must match how the entity is referenced in the text
- relationship.source and relationship.target must match entity names in this batch
- relationship.weight is your confidence (0.0-1.0)
- Skip entities with no meaningful relationships
- Prefer standard types but introduce new ones when justified

Observations:
`)

	// Calculate total narrative length for budget enforcement.
	var totalLen int
	for _, o := range obs {
		totalLen += len(o.Narrative)
	}

	// Determine proportional truncation factor.
	factor := 1.0
	if totalLen > GRAPH_EXTRACTION_CHAR_BUDGET {
		factor = float64(GRAPH_EXTRACTION_CHAR_BUDGET) / float64(totalLen)
	}

	// Write each observation, optionally truncating narrative.
	for i, o := range obs {
		idx := i + 1
		narrative := o.Narrative
		if factor < 1.0 {
			truncatedLen := int(float64(len(narrative)) * factor)
			if truncatedLen < len(narrative) && truncatedLen > 0 {
				narrative = narrative[:truncatedLen]
			}
		}

		sb.WriteString(fmt.Sprintf("[%d] Title: %s | Narrative: %s", idx, o.Title, narrative))
		if len(o.Concepts) > 0 {
			sb.WriteString(fmt.Sprintf(" | Concepts: %s", strings.Join(o.Concepts, ", ")))
		}
		if len(o.Files) > 0 {
			sb.WriteString(fmt.Sprintf(" | Files: %s", strings.Join(o.Files, ", ")))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// parsedExtractionJSON is the intermediate struct for JSON unmarshalling
// of the LLM extraction response.
type parsedExtractionJSON struct {
	Entities []struct {
		Type       string            `json:"type"`
		Name       string            `json:"name"`
		Properties map[string]string `json:"properties"`
	} `json:"entities"`
	Relationships []struct {
		Type   string  `json:"type"`
		Source string  `json:"source"`
		Target string  `json:"target"`
		Weight float64 `json:"weight"`
	} `json:"relationships"`
}

// ParseExtractionResponse parses the LLM JSON response into entities and
// relationships. Empty or missing arrays produce zero results without error.
// Malformed JSON returns an error.
func ParseExtractionResponse(response string) ([]ParsedEntity, []ParsedRelationship, error) {
	var parsed parsedExtractionJSON
	if err := json.Unmarshal([]byte(response), &parsed); err != nil {
		return nil, nil, fmt.Errorf("failed to parse extraction response: %w", err)
	}

	entities := make([]ParsedEntity, 0, len(parsed.Entities))
	for _, e := range parsed.Entities {
		if e.Name == "" || e.Type == "" {
			continue
		}
		if e.Properties == nil {
			e.Properties = make(map[string]string)
		}
		entities = append(entities, ParsedEntity{
			Type:       e.Type,
			Name:       e.Name,
			Properties: e.Properties,
		})
	}

	relationships := make([]ParsedRelationship, 0, len(parsed.Relationships))
	for _, r := range parsed.Relationships {
		if r.Type == "" || r.Source == "" || r.Target == "" {
			continue
		}
		if r.Weight < 0.0 {
			r.Weight = 0.0
		}
		if r.Weight > 1.0 {
			r.Weight = 1.0
		}
		relationships = append(relationships, ParsedRelationship{
			Type:   r.Type,
			Source: r.Source,
			Target: r.Target,
			Weight: r.Weight,
		})
	}

	return entities, relationships, nil
}

// stripMarkdownFences removes markdown code fences (```json ... ```)
// that some LLMs wrap around JSON output despite instructions to output
// raw JSON only. Returns the inner content if fences are detected, or the
// original string unchanged if no fences are present.
func stripMarkdownFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		// Find the first newline after the opening fence.
		if idx := strings.IndexByte(s, '\n'); idx != -1 {
			s = s[idx+1:]
		}
	}
	if strings.HasSuffix(s, "```") {
		s = s[:len(s)-3]
	}
	return strings.TrimSpace(s)
}

// computeEntityID computes a stable, deterministic ID for an entity based on
// its label and node type. Uses the first 16 bytes of SHA-256, hex-encoded.
func computeEntityID(label, nodeType string) string {
	hash := sha256.Sum256([]byte(label + ":" + nodeType))
	return hex.EncodeToString(hash[:16])
}

// prefixEdgeType ensures the "llm:" prefix is applied to edge types from the
// LLM. If the prefix is already present, it is not doubled.
func prefixEdgeType(edgeType string) string {
	if strings.HasPrefix(edgeType, "llm:") {
		return edgeType
	}
	return "llm:" + edgeType
}

// dedupStringSlice returns a new slice with duplicate elements removed,
// preserving order. Used to prevent observation ID array bloat when
// the same session is extracted multiple times.
func dedupStringSlice(s []string) []string {
	if len(s) <= 1 {
		return s
	}
	seen := make(map[string]bool, len(s))
	result := make([]string, 0, len(s))
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}
