# v2 Hooks Design

Finalized 2026-06-20.

## Hook Behavior Table

| Hook | v2 Behavior | Details |
|------|------------|---------|
| **SessionStart** | observe + conditional context injection | `AGENTMEMORY_INJECT_CONTEXT` controls both injection AND whether to wait for it to complete |
| **SessionEnd** | session/end + consolidate + bridge sync | End session, run consolidation pipeline, sync MEMORY.md bridge |
| **Stop** | **DELETED** | v0 bug: Stop incorrectly ran summarize + session/end on every turn. This was per-turn lifecycle ops that should only happen at session end. Deleted in v2 — no replacement |
| **UserPromptSubmit** | observe (user prompt) | Record what the user asked |
| **PreToolUse** | conditional enrich context injection | `AGENTMEMORY_INJECT_CONTEXT` must be enabled. Runs file-specific search for context relevant to the tool about to be used |
| **PostToolUse** | observe (tool_name, tool_input, tool_output, image_data) | Record tool execution results. Image data (base64 inline) is extracted and saved to filesystem at `~/.agentmemory/images/<sha256>.ext` — only the file path is stored in `observations.image_path` |
| **PostToolUseFailure** | observe (tool_name, tool_input, error), skip interrupt | Record failure but do NOT interrupt agent flow. Errors are informational |
| **PreCompact** | conditional context injection | `AGENTMEMORY_INJECT_CONTEXT` must be enabled. Runs before context window compaction to ensure memory is fresh |
| **SubagentStart** | observe (agent_id, agent_type), **NO context injection** | Subagent tasks are narrow and focused — adding 1500 tokens of general context would waste tokens and distract the subagent |
| **SubagentStop** | observe (agent_id, agent_type, last_message) | Record subagent completion and its final output |
| **Notification** | observe (filter: permission_prompt → notification_type, title, message) | Only record notification-type events, filter out other permission prompts |
| **TaskCompleted** | observe (task_id, task_subject, task_description) | Record when a tracked task completes |
| **PostCommit** | session/commit (git sha → session link, NO observe) | Link git commit to session for commit-context queries. Does NOT create an observation |

## Key Design Decisions

### 1. Only Three Hooks Inject Context
- SessionStart — initial load
- PreToolUse — enriched, file-specific context
- PreCompact — refresh before compaction

This is a deliberate constraint. Every context injection costs tokens.
v0 had context injection disabled by default since v0.8.10 to avoid extra token burn.

### 2. SubagentStart Does NOT Inject Context
Subagents handle narrow, specific tasks. Injecting 1500 tokens of general project
context would:
- Waste tokens on irrelevant information
- Distract the subagent from its focused task
- Add latency to subagent startup

### 3. Stop Hook Deleted
The v0 Stop hook fired on every turn and ran summarize + session/end.
This was architecturally wrong:
- Summarization should happen at session end, not per-turn
- Session/end should happen once, not on every agent stop
- Per-turn lifecycle ops belong elsewhere (if at all)

v2 simply deletes this hook. No replacement.

### 4. All Recording Hooks Only Observe
Hooks that record data (PostToolUse, UserPromptSubmit, Notification, etc.)
only call observe — they have no side effects beyond storing the observation.
This keeps hooks predictable and fast:
- No consolidation during a hook
- No search during a hook
- No context injection during a recording hook
- Hook latency is bounded by a single DB insert

---

## Protocol Architecture

```
Plugin Hook Scripts (shell)
  ↓ POST /v1/api/observe
  ↓ POST /v1/api/session/end
  ↓ GET  /v1/api/context?files=...
REST API handlers

Agent MCP Tools
  ↓ /v1/mcp (Streamable HTTP)
MCP Tool handlers
```

Hooks and MCP tools go through DIFFERENT channels:
- **Hooks → REST API** — shell scripts call HTTP endpoints. Simple, debuggable, no MCP overhead.
- **MCP tools → MCP Streamable HTTP** — agents use the MCP protocol. Full tool discovery and streaming.

---

## Hook Scripts Location

Hook scripts are shell scripts that the agent platform invokes at lifecycle events.
They call the agentmemory REST API. Example:

```bash
#!/bin/bash
# PostToolUse hook
curl -s -X POST "$AGENTMEMORY_URL/v1/api/observe" \
  -H "Authorization: Bearer $AGENTMEMORY_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"hook_type":"PostToolUse","tool_name":"'$TOOL_NAME'",...}'
```

All logic lives in the Go REST handlers. Hook scripts are thin HTTP clients.
