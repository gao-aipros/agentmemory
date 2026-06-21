-- name: InsertGraphNode :one
INSERT INTO graph_nodes (id, node_type, entity_id, label, metadata)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetGraphNode :one
SELECT * FROM graph_nodes WHERE id = $1;

-- name: ListGraphNodesByType :many
SELECT * FROM graph_nodes WHERE node_type = $1 ORDER BY created_at DESC;

-- name: ListGraphNodesByEntity :many
SELECT * FROM graph_nodes WHERE entity_id = $1;

-- name: DeleteGraphNode :exec
DELETE FROM graph_nodes WHERE id = $1;
