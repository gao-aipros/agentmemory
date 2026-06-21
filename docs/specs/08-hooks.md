# v2 Hooks Design

Finalized 2026-06-20.

## Hook Behavior Table

| Hook | v2 Behavior |
|------|------------|
| **SessionStart** | observe + conditional context injection (`AGENTMEMORY_INJECT_CONTEXT` controls inject + wait) |
| **SessionEnd** | session/end + consolidate + bridge sync |
| **Stop** | **Deleted** (v0 incorrectly does summarize + session/end per-turn) |
| **UserPromptSubmit** | observe (record user prompt) |
| **PreToolUse** | conditional enrich context injection (`AGENTMEMORY_INJECT_CONTEXT`, file-specific search) |
| **PostToolUse** | observe (tool_name, tool_input, tool_output, image_data) |
| **PostToolUseFailure** | observe (tool_name, tool_input, error), skip interrupt |
| **PreCompact** | conditional context injection (`AGENTMEMORY_INJECT_CONTEXT`) |
| **SubagentStart** | observe (agent_id, agent_type), **no context injection** |
| **SubagentStop** | observe (agent_id, agent_type, last_message) |
| **Notification** | observe (filter permission_prompt: notification_type, title, message) |
| **TaskCompleted** | observe (task_id, task_subject, task_description) |
| **PostCommit** | session/commit (git sha → session link, no observe) |

## Key Decisions

- **Only three hooks inject context:** SessionStart, PreToolUse (enrich), PreCompact
- **SubagentStart in v2 does NOT inject context** (subagent tasks are narrow)
- **Stop hook deleted** — v0 incorrectly does per-turn lifecycle ops
- **All recording hooks only observe, no side effects**

## Protocol

- Hooks → REST API (plugin hook scripts call REST endpoints)
- MCP tools → MCP Streamable HTTP
