-- name: UpsertSessionSummary :one
INSERT INTO session_summaries (id, session_id, visibility, summary_text, concepts)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (session_id) DO UPDATE SET
    summary_text = EXCLUDED.summary_text,
    concepts = EXCLUDED.concepts,
    visibility = EXCLUDED.visibility
RETURNING *;

-- name: GetSessionSummary :one
SELECT * FROM session_summaries WHERE session_id = $1;
