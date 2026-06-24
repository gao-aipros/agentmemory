-- name: BatchInsertLessons :copyfrom
INSERT INTO lessons (id, team_id, visibility, content, context, confidence, source)
VALUES ($1, $2, $3, $4, $5, $6, $7);
