-- name: AddTeamMember :one
INSERT INTO team_members (id, team_id, user_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: RemoveTeamMember :exec
DELETE FROM team_members WHERE team_id = $1 AND user_id = $2;

-- name: ListTeamMembers :many
SELECT * FROM team_members WHERE team_id = $1 ORDER BY joined_at;

-- name: GetUserTeam :one
SELECT t.* FROM teams t
JOIN team_members tm ON t.id = tm.team_id
WHERE tm.user_id = $1
LIMIT 1;
