-- AgentMemory v2 — Full Schema Rollback (consolidated)
-- Drop order respects FK dependencies (children first).

DROP TABLE IF EXISTS procedural_memories CASCADE;
DROP TABLE IF EXISTS insights CASCADE;
DROP TABLE IF EXISTS crystals CASCADE;
DROP TABLE IF EXISTS graph_edges CASCADE;
DROP TABLE IF EXISTS graph_nodes CASCADE;
DROP TABLE IF EXISTS lesson_reinforcements CASCADE;
DROP TABLE IF EXISTS lessons CASCADE;
DROP TABLE IF EXISTS memories CASCADE;
DROP TABLE IF EXISTS session_summaries CASCADE;
DROP TABLE IF EXISTS compressed_embeddings CASCADE;
DROP TABLE IF EXISTS compressed_observations CASCADE;
DROP TABLE IF EXISTS observation_embeddings CASCADE;
DROP TABLE IF EXISTS observations CASCADE;
DROP TABLE IF EXISTS sessions CASCADE;
DROP TABLE IF EXISTS team_members CASCADE;
DROP TABLE IF EXISTS teams CASCADE;
DROP TABLE IF EXISTS api_keys CASCADE;
DROP TABLE IF EXISTS users CASCADE;

-- Extensions (optional — only drop if no other database uses vector)
-- DROP EXTENSION IF EXISTS vector;

-- Functions
DROP FUNCTION IF EXISTS hybrid_search(text, vector, int, text);
DROP FUNCTION IF EXISTS bm25_search(text, int, text);
