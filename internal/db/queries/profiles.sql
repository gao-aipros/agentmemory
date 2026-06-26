-- name: UpsertProfile :exec
INSERT INTO project_profiles (project_slug, top_concepts, top_files, conventions, common_errors)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (project_slug) DO UPDATE SET
    top_concepts = EXCLUDED.top_concepts,
    top_files = EXCLUDED.top_files,
    conventions = EXCLUDED.conventions,
    common_errors = EXCLUDED.common_errors,
    updated_at = now();

-- name: GetProfile :one
SELECT * FROM project_profiles WHERE project_slug = $1;

-- name: DeleteProfile :exec
DELETE FROM project_profiles WHERE project_slug = $1;
