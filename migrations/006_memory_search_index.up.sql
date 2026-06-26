-- AgentMemory v2 — Memory Search Indexes
-- Adds vector embedding storage and BM25 full-text search for the memories table.

-- ===========================================================================
-- Memory Embeddings (vector search)
-- ===========================================================================
CREATE TABLE IF NOT EXISTS memory_embeddings (
    id TEXT PRIMARY KEY,
    memory_id TEXT NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    embedding vector(1536) NOT NULL,
    model TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- HNSW approximate nearest neighbor index on memory embeddings (per-model partial)
CREATE INDEX IF NOT EXISTS idx_mem_emb_hnsw_ada002
    ON memory_embeddings USING hnsw (embedding vector_cosine_ops)
    WHERE model = 'text-embedding-ada-002';

-- ===========================================================================
-- BM25 Full-Text Search (ParadeDB) for memories
-- ===========================================================================
CREATE INDEX IF NOT EXISTS idx_memories_bm25 ON memories
USING bm25 (id, content)
WITH (key_field='id');
