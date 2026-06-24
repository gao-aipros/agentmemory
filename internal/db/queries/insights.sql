-- name: InsertInsight :exec
INSERT INTO insights (id, content, confidence, source, created_at)
VALUES ($1, $2, $3, 'reflect', now());
