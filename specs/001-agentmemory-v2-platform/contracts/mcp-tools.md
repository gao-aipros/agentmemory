# MCP Tool Contracts: AgentMemory v2

**Feature**: AgentMemory v2 Platform Migration
**Date**: 2026-06-21
**Source**: v0 tool registry `src/mcp/tools-registry.ts` + v2 design docs

## Protocol

- **Transport**: MCP Streamable HTTP at `/v1/mcp`
- **Auth**: `Authorization: Bearer st_<jwt>` or `Bearer ak_<api_key>`
- **Response**: JSON (`JSONResponse: true`)
- **SDK**: `modelcontextprotocol/go-sdk`

## Tool Categories

### Memory Operations (Core Pipeline)

| Tool | Parameters | Returns | Description |
|------|-----------|---------|-------------|
| `memory_observe` | `type`, `title`, `narrative`, `facts?`, `concepts?`, `files?`, `importance?` | `{observation_id}` | Record a raw observation |
| `memory_save` | `content`, `type?`, `concepts?`, `files?`, `project?` | `{memory_id}` | Save explicit memory/insight |
| `memory_recall` | `query`, `limit?`, `format?` | `{observations[]}` | Search past observations |
| `memory_smart_search` | `query`, `limit?`, `expand_ids?` | `{results[]}` | Hybrid search with progressive disclosure |
| `memory_forget` | `observation_ids` | `{deleted_count}` | Delete specific observations |
| `memory_compress_file` | `file_path` | `{original_size, compressed_size}` | Compress a markdown file |

### Session Operations

| Tool | Parameters | Returns | Description |
|------|-----------|---------|-------------|
| `memory_sessions` | — | `{sessions[]}` | List recent sessions |
| `memory_timeline` | `anchor`, `before?`, `after?`, `project?` | `{observations[]}` | Chronological observations around anchor |
| `memory_handoff` | — | `{context}` | Resume most recent session |
| `memory_recap` | — | `{recap}` | Summarize last N sessions |

### Lesson Operations

| Tool | Parameters | Returns | Description |
|------|-----------|---------|-------------|
| `memory_lesson_save` | `content`, `context?`, `confidence?`, `tags?`, `project?` | `{lesson_id}` | Save a lesson learned |
| `memory_lesson_recall` | `query`, `limit?`, `min_confidence?`, `project?` | `{lessons[]}` | Search lessons |

### Team Operations (New in v2)

| Tool | Parameters | Returns | Description |
|------|-----------|---------|-------------|
| `team_create` | `name`, `default_visibility?` | `{team_id, name}` | Create a team |
| `team_delete` | `team_id` | `{deleted: true}` | Delete own team (owner only) |
| `team_add_member` | `team_id`, `user_id` | `{member_id}` | Add member to team |
| `team_remove_member` | `team_id`, `user_id` | `{removed: true}` | Remove member |
| `team_list_members` | `team_id` | `{members[]}` | List team members |
| `team_feed` | `limit?` | `{items[]}` | Recent shared team items |

### Auth Operations (New in v2)

| Tool | Parameters | Returns | Description |
|------|-----------|---------|-------------|
| `auth_create_key` | `label`, `expires_at?` | `{key_id, key, prefix}` | Generate API key (`ak_` prefix) |
| `auth_list_keys` | — | `{keys[]}` | List own API keys |
| `auth_revoke_key` | `key_id` | `{revoked: true}` | Revoke API key |

### Action Operations

| Tool | Parameters | Returns | Description |
|------|-----------|---------|-------------|
| `memory_action_create` | `title`, `description?`, `priority?`, `tags?`, `requires?`, `parent_id?`, `project?` | `{action_id}` | Create action item |
| `memory_action_update` | `action_id`, `status?`, `priority?`, `result?` | `{action}` | Update action status |
| `memory_frontier` | `project?`, `agent_id?`, `limit?` | `{actions[]}` | Get unblocked actions |
| `memory_next` | `project?`, `agent_id?` | `{action}` | Single next action to work on |

### Pipeline Operations

| Tool | Parameters | Returns | Description |
|------|-----------|---------|-------------|
| `memory_consolidate` | `tier?` | `{consolidated}` | Run consolidation pipeline |
| `memory_crystallize` | `action_ids` | `{crystal_id}` | Crystalize completed actions |
| `memory_reflect` | `project?`, `max_clusters?` | `{insights[]}` | Reflect and synthesize insights |
| `memory_diagnose` | `categories?` | `{issues[]}` | Health check across subsystems |
| `memory_heal` | `categories?`, `dry_run?` | `{fixed[]}` | Auto-fix diagnostic issues |

### Governance & Verification

| Tool | Parameters | Returns | Description |
|------|-----------|---------|-------------|
| `memory_verify` | `id` | `{provenance}` | Verify memory citation chain |
| `memory_audit` | `operation?`, `limit?` | `{entries[]}` | Audit trail of operations |

### Export & Sync

| Tool | Parameters | Returns | Description |
|------|-----------|---------|-------------|
| `memory_export` | — | `{data}` | Export all memory as JSON |
| `memory_obsidian_export` | `types?`, `vault_dir?` | `{files[]}` | Export as Obsidian vault |
| `memory_commit_lookup` | `sha` | `{commit, session}` | Look up git commit session |
| `memory_commits` | `branch?`, `repo?`, `limit?` | `{commits[]}` | List agent-linked commits |
| `memory_mesh_sync` | `direction?`, `peer_id?` | `{synced}` | Sync with peer instances |

### Graph Operations

| Tool | Parameters | Returns | Description |
|------|-----------|---------|-------------|
| `memory_graph_query` | `query?`, `node_type?`, `start_node_id?`, `max_depth?` | `{nodes[], edges[]}` | Query knowledge graph |
| `memory_relations` | `memory_id`, `max_hops?`, `min_confidence?` | `{relations[]}` | Find memory relationships |

### Context & Team

| Tool | Parameters | Returns | Description |
|------|-----------|---------|-------------|
| `memory_profile` | `project`, `refresh?` | `{profile}` | Project profile with top concepts |
| `memory_patterns` | `project?` | `{patterns[]}` | Detect recurring patterns |
| `memory_facet_query` | `match_all?`, `match_any?`, `target_type?` | `{results[]}` | Query by facet tags |
| `memory_facet_tag` | `target_id`, `target_type`, `dimension`, `value` | `{tagged}` | Attach structured tag |
| `memory_vision_search` | `query_text?`, `query_image_ref?`, `session_id?`, `top_k?` | `{matches[]}` | Cross-modal image search |

### Slots & Working Memory

| Tool | Parameters | Returns | Description |
|------|-----------|---------|-------------|
| `memory_slot_create` | `label`, `content?`, `description?`, `scope?`, `size_limit?`, `pinned?`, `project?` | `{slot}` | Create a memory slot |
| `memory_slot_get` | `label`, `project?` | `{content, metadata}` | Read a slot by label |
| `memory_slot_list` | `project?` | `{slots[]}` | List all slots |
| `memory_slot_replace` | `label`, `content`, `project?` | `{slot}` | Replace slot content |
| `memory_slot_delete` | `label`, `project?` | `{deleted}` | Delete a slot |
| `memory_slot_append` | `label`, `text`, `project?` | `{slot}` | Append text to a slot |

### Signals, Sentinels & Checkpoints

| Tool | Parameters | Returns | Description |
|------|-----------|---------|-------------|
| `memory_signal_send` | `from`, `content`, `to?`, `type?`, `reply_to?` | `{signal_id}` | Send message to agent |
| `memory_signal_read` | `agent_id`, `thread_id?`, `limit?`, `unread_only?` | `{messages[]}` | Read messages for agent |
| `memory_sentinel_create` | `name`, `type`, `config?`, `linked_action_ids?`, `expires_in_ms?` | `{sentinel_id}` | Create event-driven sentinel |
| `memory_sentinel_trigger` | `sentinel_id`, `result?` | `{triggered}` | Fire a sentinel |
| `memory_checkpoint` | `operation`, `name?`, `type?`, `checkpoint_id?`, `status?`, `linked_action_ids?` | `{checkpoint}` | Create/resolve/list checkpoints |

### Sketches, Routines & Snapshots

| Tool | Parameters | Returns | Description |
|------|-----------|---------|-------------|
| `memory_sketch_create` | `title`, `description?`, `project?`, `expires_in_ms?` | `{sketch_id}` | Create ephemeral action graph |
| `memory_sketch_promote` | `sketch_id`, `project?` | `{actions[]}` | Promote sketch to permanent actions |
| `memory_routine_run` | `routine_id`, `project?`, `initiated_by?` | `{run_id}` | Instantiate frozen workflow routine |
| `memory_snapshot_create` | `message?` | `{snapshot_id}` | Git-versioned memory snapshot |
| `memory_file_history` | `files`, `session_id?` | `{observations[]}` | Past observations about files |

### Other v1 Service Tools

| Tool | Parameters | Returns | Description |
|------|-----------|---------|-------------|
| `memory_lease` | `action_id`, `agent_id`, `operation`, `result?`, `ttl_ms?` | `{lease}` | Acquire/release/renew action lease |
| `memory_insight_list` | `limit?`, `min_confidence?`, `project?` | `{insights[]}` | List synthesized insights |
| `memory_team_share` | `item_id`, `item_type` | `{shared}` | Share memory with team |
| `memory_claude_bridge_sync` | `direction` | `{synced}` | Sync to/from CLAUDE.md |
| `memory_commit_lookup` (moved from Export) | `sha` | `{commit, session}` | Look up commit session |

## Tool Count Summary

| Category | Count |
|----------|-------|
| Memory Operations | 6 |
| Session Operations | 4 |
| Lesson Operations | 2 |
| Team Operations | 6 |
| Auth Operations | 3 |
| Action Operations | 4 |
| Pipeline Operations | 5 |
| Governance | 2 |
| Export & Sync | 5 |
| Graph Operations | 2 |
| Context & Misc | 5 |
| Slots & Working Memory | 6 |
| Signals, Sentinels & Checkpoints | 5 |
| Sketches, Routines & Snapshots | 5 |
| Other v1 Services | 5 |
| **Total v2** | **55** |

Note: The original v1 had 51 tools total. v2 exposes 55 public MCP tools
(44 original v1 public + 11 v1 service tools now exposed as MCP tools:
slots, signals, sentinels, checkpoints, sketches, routines, snapshots,
file-history, lease, insight-list, team-share, bridge-sync).
