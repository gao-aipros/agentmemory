ALTER TABLE graph_nodes ADD COLUMN IF NOT EXISTS source_obs_ids TEXT[] DEFAULT '{}';
ALTER TABLE graph_edges ADD COLUMN IF NOT EXISTS source_obs_ids TEXT[] DEFAULT '{}';

-- UNIQUE index for entity dedup: same name + same type = same node
CREATE UNIQUE INDEX IF NOT EXISTS idx_graph_nodes_label_type
    ON graph_nodes (label, node_type);

-- UNIQUE index for edge dedup: same source + same target + same type = same edge
CREATE UNIQUE INDEX IF NOT EXISTS idx_graph_edges_triple
    ON graph_edges (from_node_id, to_node_id, edge_type);
