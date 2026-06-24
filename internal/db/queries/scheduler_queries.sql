-- name: ListSessionsWithUncompressedObservations :many
SELECT DISTINCT session_id FROM observations
WHERE compressed_at IS NULL AND session_id IS NOT NULL
ORDER BY session_id;

-- name: ClaimUncompressedObservations :many
SELECT id, title, narrative, facts, concepts, session_id
FROM observations
WHERE session_id = $1 AND compressed_at IS NULL
ORDER BY created_at
FOR UPDATE SKIP LOCKED;

-- name: ListSessionsNeedingSummarization :many
SELECT s.id FROM sessions s
JOIN observations o ON o.session_id = s.id
LEFT JOIN session_summaries ss ON ss.session_id = s.id
WHERE o.compressed_at IS NOT NULL
  AND (ss.created_at IS NULL OR o.compressed_at > ss.created_at)
GROUP BY s.id, ss.created_at;

-- name: ListUnconsolidatedSessions :many
SELECT ss.session_id FROM session_summaries ss
WHERE NOT EXISTS (
    SELECT 1 FROM memories m
    WHERE m.source = 'consolidation' AND m.created_at > ss.created_at
);

-- name: HasUnreflectedMemories :one
SELECT COUNT(*) > 0 AS has_unreflected FROM memories m
WHERE NOT EXISTS (
    SELECT 1 FROM insights i
    WHERE i.source = 'reflect' AND i.created_at > m.created_at
);
