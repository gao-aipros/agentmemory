-- name: ListEvictionCandidates :many
SELECT id, importance,
       EXTRACT(EPOCH FROM (now() - created_at)) / 86400 AS age_days
FROM observations
WHERE importance < $1
ORDER BY importance ASC, created_at ASC
LIMIT $2;

-- name: GetFileHistory :many
SELECT id, session_id, title, narrative, files, timestamp
FROM observations
WHERE files IS NOT NULL AND files && $1
  AND session_id != $2
ORDER BY timestamp DESC
LIMIT 100;

-- name: GetConceptFrequencies :many
SELECT unnest(concepts) AS concept, count(*) AS freq
FROM observations
WHERE concepts IS NOT NULL AND cardinality(concepts) > 0
GROUP BY concept
ORDER BY freq DESC
LIMIT 50;
