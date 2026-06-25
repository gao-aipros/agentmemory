-- name: InsertGraphEdge :one
INSERT INTO graph_edges (id, from_node_id, to_node_id, edge_type, weight)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetGraphEdge :one
SELECT * FROM graph_edges WHERE id = $1;

-- name: ListGraphEdgesFrom :many
SELECT * FROM graph_edges WHERE from_node_id = $1 ORDER BY created_at DESC;

-- name: ListGraphEdgesTo :many
SELECT * FROM graph_edges WHERE to_node_id = $1 ORDER BY created_at DESC;

-- name: DeleteGraphEdge :exec
DELETE FROM graph_edges WHERE id = $1;

-- name: UpsertGraphEdge :one
INSERT INTO graph_edges (id, from_node_id, to_node_id, edge_type, weight, source_obs_ids)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (from_node_id, to_node_id, edge_type) DO UPDATE SET
    weight = (graph_edges.weight + EXCLUDED.weight) / 2.0,
    source_obs_ids = graph_edges.source_obs_ids || EXCLUDED.source_obs_ids
RETURNING *;
