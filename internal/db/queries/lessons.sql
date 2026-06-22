-- name: InsertLesson :one
INSERT INTO lessons (id, team_id, visibility, content, context, confidence, source)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetLesson :one
SELECT * FROM lessons WHERE id = $1;

-- name: ListLessonsByTeam :many
SELECT * FROM lessons WHERE team_id = $1 ORDER BY created_at DESC;

-- name: UpdateConfidence :exec
UPDATE lessons SET confidence = $2, last_reinforced_at = now() WHERE id = $1;

-- name: DeleteLesson :exec
DELETE FROM lessons WHERE id = $1;

-- name: InsertLessonReinforcement :exec
INSERT INTO lesson_reinforcements (id, lesson_id, observation_id, confidence_delta)
VALUES ($1, $2, $3, $4);

-- name: ListAllLessons :many
-- sqlc.narg('team_id') enforces cross-tenant isolation.
SELECT * FROM lessons
WHERE (sqlc.narg('team_id')::text IS NULL OR team_id = sqlc.narg('team_id'))
ORDER BY created_at DESC
LIMIT $1;
