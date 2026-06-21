# v2 Smart Search Design

Finalized 2026-06-20.

## Core Change

v0 three-way independent search (JS in-memory Map) + JS RRF merge
→ v2 **single SQL FULL OUTER JOIN**.

## Execution Flow

```
1. embed(query) → Float32Array
   (external embedding API call, unchanged from v0)

2. Single SQL(query_text, query_vec):
   ┌─────────────────────────────────────────┐
   │ ParadeDB pg_search BM25 match(query_text) │ → scores_bm25
   │ pgvector HNSW <-> (query_vec, cosine)    │ → scores_vector
   │ WITH RECURSIVE graph traversal            │ → scores_graph
   │   (independent of vector top-5)           │
   └─────────────────────────────────────────┘

3. FULL OUTER JOIN weighted merge
   + ORDER BY combined_score DESC
   + LIMIT N
```

## Weights

| Dimension | Weight | Index |
|-----------|--------|-------|
| BM25 | 0.4 | ParadeDB `USING bm25 (id, title, narrative, facts) WITH (key_field = 'id')` |
| Vector | 0.6 | pgvector `USING hnsw (embedding vector_cosine_ops)` — partial index per active model |
| Graph | 0.3 | `WITH RECURSIVE` on `graph_nodes`/`graph_edges` tables |

Weights unchanged from v0.

## Key Decisions

### HNSW Index Structure
- Cosine distance metric preserved from v0 — HNSW only changes the INDEX STRUCTURE
- Effect: O(n) brute-force vector search → O(log n) approximate nearest neighbor (ANN)
- The embedding model, dimension, and distance function are unchanged
- Partial index per active embedding model (supports multiple models simultaneously)

### Graph Traversal Independence
- In v0, graph expansion started from top-5 vector results — coupling graph quality to vector quality
- In v2, graph traversal is independent via `WITH RECURSIVE` CTE
- Graph nodes/edges live in dedicated PG tables (`graph_nodes`, `graph_edges`)
- No longer limited to expanding from vector results

### Top-N Only at Final Step
- Each search stream (BM25, vector, graph) returns ALL matches above threshold
- Weighted merge + ORDER BY + LIMIT N happens only at the final SELECT
- No per-stream over-fetch required
- This is a correctness improvement over v0 where per-stream limiting could drop relevant results

### BM25 Index
- ParadeDB's `USING bm25` creates a full-text search index on the observation table
- Indexed columns: `id` (key field), `title`, `narrative`, `facts`
- Query: ParadeDB's `match()` function with the query text directly in SQL

## Why Single SQL

v0's three-way search required:
1. Three independent JS Map lookups (BM25 index, vector index, graph adjacency)
2. JavaScript RRF (Reciprocal Rank Fusion) merge in application code
3. Data serialization between DB and app for each stream

v2's single SQL:
1. One query, one round trip
2. Merge happens inside PostgreSQL — no application-level fusion
3. All indexes queried simultaneously — PostgreSQL can parallelize
4. Results already sorted and limited — no post-processing
