# v2 Smart Search Design

**Core change:** v0 three-way independent search (JS in-memory Map) + JS RRF merge
→ v2 single SQL FULL OUTER JOIN.

## Execution Flow

1. `embed(query)` → Float32Array (external API, unchanged)
2. Single SQL(query_text, query_vec):
   - ParadeDB `pg_search` BM25 match(query_text)
   - pgvector HNSW vector cosine distance
   - `WITH RECURSIVE` graph traversal (independent of vector top-5)
3. FULL OUTER JOIN weighted merge + ORDER BY + LIMIT N

## Weights

- BM25: 0.4
- Vector: 0.6
- Graph: 0.3

(Same as v0.)

## Key Decisions

- HNSW is the index structure, distance metric still cosine distance (O(n) brute → O(log n) ANN)
- Top-N only at the end, no per-stream over-fetch
- Graph independent of vector, no longer expanding from top-5 vector results
- HNSW index on observation_embeddings, partial index per active model
- BM25 via ParadeDB `USING bm25` on observations(id, title, narrative, facts)
