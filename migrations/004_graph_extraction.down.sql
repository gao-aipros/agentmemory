DROP INDEX IF EXISTS idx_graph_nodes_label_type;
DROP INDEX IF EXISTS idx_graph_edges_triple;
ALTER TABLE graph_nodes DROP COLUMN IF EXISTS source_obs_ids;
ALTER TABLE graph_edges DROP COLUMN IF EXISTS source_obs_ids;
