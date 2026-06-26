-- AgentMemory v2 — Memory Search Indexes (rollback)

DROP TABLE IF EXISTS memory_embeddings CASCADE;

DROP INDEX IF EXISTS idx_memories_bm25;
