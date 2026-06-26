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

-- name: UpsertGraphNode :one
INSERT INTO graph_nodes (id, node_type, entity_id, label, metadata, source_obs_ids)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (label, node_type) DO UPDATE SET
    source_obs_ids = graph_nodes.source_obs_ids || EXCLUDED.source_obs_ids,
    metadata = EXCLUDED.metadata
RETURNING *;
