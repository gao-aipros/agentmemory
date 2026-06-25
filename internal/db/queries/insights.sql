-- name: UpsertInsight :exec
INSERT INTO insights (id, title, content, confidence, source_concept_cluster,
    source_memory_ids, source_lesson_ids, project, tags, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now(), now())
ON CONFLICT (id) DO UPDATE SET
    deleted = false,
    confidence = LEAST(1.0, insights.confidence + (1.0 - insights.confidence) * 0.10),
    reinforcement_count = insights.reinforcement_count + 1,
    last_reinforced_at = now(),
    title = EXCLUDED.title,
    content = EXCLUDED.content,
    source_concept_cluster = EXCLUDED.source_concept_cluster,
    source_memory_ids = EXCLUDED.source_memory_ids,
    source_lesson_ids = EXCLUDED.source_lesson_ids,
    tags = EXCLUDED.tags,
    updated_at = now();

-- name: ListInsights :many
SELECT id, title, content, confidence, reinforcement_count, project, tags, created_at, updated_at
FROM insights
WHERE deleted = false
  AND (sqlc.narg('project')::text IS NULL OR project = sqlc.narg('project'))
  AND (sqlc.narg('min_confidence')::float IS NULL OR confidence >= sqlc.narg('min_confidence')::float)
ORDER BY confidence DESC
LIMIT $1;

-- name: SearchInsights :many
SELECT id, title, content, confidence, reinforcement_count, project, tags, created_at, updated_at
FROM insights
WHERE deleted = false
  AND (sqlc.narg('project')::text IS NULL OR project = sqlc.narg('project'))
  AND (sqlc.narg('min_confidence')::float IS NULL OR confidence >= sqlc.narg('min_confidence')::float)
  AND (sqlc.narg('query')::text IS NULL OR to_tsvector('english', title || ' ' || content) @@ plainto_tsquery('english', sqlc.narg('query')))
ORDER BY confidence DESC, ts_rank(to_tsvector('english', title || ' ' || content), plainto_tsquery('english', sqlc.narg('query'))) DESC
LIMIT $1;

-- name: MarkMemoriesReflected :exec
UPDATE memories SET reflected = true, updated_at = now()
WHERE id = ANY($1::text[]);

-- name: ApplyDecayWithCounts :one
WITH decayed AS (
    UPDATE insights SET
        confidence = GREATEST(0.05, confidence - decay_rate * $1::float),
        last_decayed_at = now(),
        deleted = CASE
            WHEN confidence - decay_rate * $1::float <= 0.1 AND reinforcement_count = 0
            THEN true ELSE deleted
        END,
        updated_at = now()
    WHERE deleted = false
      AND (
        last_reinforced_at IS NULL
        OR last_reinforced_at < now() - INTERVAL '1 week'
      )
    RETURNING id, deleted
)
SELECT
    COUNT(*) FILTER (WHERE NOT deleted)::int AS decayed_count,
    COUNT(*) FILTER (WHERE deleted)::int AS soft_deleted_count
FROM decayed;
