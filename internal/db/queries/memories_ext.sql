-- name: ListAllMemories :many
SELECT * FROM memories WHERE reflected = false AND source = 'consolidation' AND deleted = false ORDER BY created_at DESC LIMIT $1;

-- name: BatchInsertMemories :copyfrom
INSERT INTO memories (id, owner_type, owner_user_id, owner_team_id, visibility, content, concepts, source, confidence)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);
