# v2 Pipeline Architecture

Preserved from v0. Finalized 2026-06-21.

## Pipeline Stages

```
observe → compress → summarize → consolidate → reflect
```

This is the core multi-level extract/aggregate/consolidation pipeline from v0,
carried forward unchanged in structure.

## Dual Pipelines

v0 has two parallel pipelines that v2 preserves:

1. **Observation pipeline** — captures what happened (tool calls, prompts, responses)
2. **Action pipeline** — tracks what needs to be done (tasks, leases, checkpoints)

Both pipelines feed into the same storage and search infrastructure.

## Dual Consumption Lines

1. **Context line** — injects relevant memory into agent sessions (context injection)
2. **Search line** — powers memory_recall, smart_search, and other query tools

These are separate consumption paths optimized for different use cases:
context injection needs tight token budgets; search can return richer results.

## Data Flow per Stage

| Stage | Input | Output | Destination |
|-------|-------|--------|-------------|
| **observe** | Hook event (tool call, prompt, etc.) | Raw observation | Session store (`observations` table) |
| **compress** | Raw observation | CompressedObservation | BM25 + Vector dual index (`observations` + `observation_embeddings`) |
| **summarize** | All session observations | SessionSummary | Context injection ONLY (NOT in search index) |
| **consolidate** | SessionSummary | SemanticMemory (facts) | reflect + retention (NOT in search index) |
| **consolidate** | Memory (pattern type) | ProceduralMemory | procedural store (`procedural_memories` table) |
| **consolidate** | Graph relationships | Insights | insights store (`insights` table) |
| **reflect** | SemanticMemory + Insights | Higher-order insights | insights reinforcement + decay |

### Key Rules

- **SessionSummary goes to context injection only** — it is deliberately excluded from the search index to avoid polluting search results with session-level noise
- **CompressedObservation goes to BM25+Vector dual index** — this is the primary searchable memory unit
- **SemanticMemory is consumed by reflect + retention** — it feeds higher-order reasoning but is NOT directly searchable
- **Consolidation inputs are typed:**
  - SessionSummary → SemanticMemory (individual facts)
  - Memory (pattern type) → ProceduralMemory (workflows, procedures)
  - Graph relationships → Insights (concept clusters)

---

## Context Injection

### Hard Limit
1500 tokens total — enforced at assembly time, not per-source.

### Sources and Budgets

| Source | Budget | Content |
|--------|--------|---------|
| Pinned Slots | ~300t | Project-level pinned slots (persona, guidance, pending_items, etc.) |
| Your Recent Sessions | ~250t | User's own recent session summaries, most recent first |
| Team Lessons | ~200t | Team lessons sorted by confidence, not deleted |
| Team Shared Memory | ~250t | Team-visible consolidated memories, latest versions |
| Project Profile | ~100t | Project-level patterns: top concepts, common errors, conventions |

### Format
Each item: **one-line summary + recall ID**.
The agent sees a compact reference list. If an item looks relevant, the agent
can fetch the full content via `memory_recall` with the recall ID.

### Activation
Controlled by `AGENTMEMORY_INJECT_CONTEXT` env var.
Only three hooks inject context:
- **SessionStart** — initial context load
- **PreToolUse** — enriched context (file-specific search before tool execution)
- **PreCompact** — context refresh before context window compaction

SubagentStart does NOT inject context (subagent tasks are narrow, adding context
would waste tokens on irrelevant information).

---

## Implementation Rule: v0 as Living Spec

v0 is v2's **behavioral spec**, not a code template.

### What This Means
- When unsure how something should work: **read the v0 source**
- Understand: what functions do, how the pipeline connects, how data flows
- Reference behavior semantics, NOT code structure
- TS → Go is an entirely different implementation — copy the WHAT, not the HOW

### v0 Source
https://github.com/Noodle05/agentmemory

### Key Files to Reference
- `src/functions/observe.ts` — observation capture logic
- `src/functions/compress.ts` — compression/extraction logic
- `src/functions/summarize.ts` — session summarization
- `src/functions/consolidate.ts` — consolidation pipeline
- `src/functions/context.ts` — context injection assembly
- `src/functions/team.ts` — team sharing logic (v0 reference, v2 redesigns this)
- `src/types.ts` — data type definitions
- `src/config.ts` — configuration surface

---

## Execution Model (Async/Sync)

| Stage | Execution | Rationale |
|-------|-----------|-----------|
| **observe** | Synchronous | Fast — single DB insert. Hook needs immediate ack. |
| **compress** | Async (goroutine) | May call LLM. Non-blocking — observe returns before compress completes. |
| **summarize** | Async (triggered by SessionEnd) | Runs once per session. Not on the critical path. |
| **consolidate** | Async (triggered by SessionEnd) | May call LLM, may take minutes. Runs as background job. |
| **reflect** | Async (scheduled/timer) | Periodically reinforces or decays insights. No caller waiting. |
| **context injection** | Synchronous | Must complete before agent receives context. Bounded by 1500-token assembly. |

No external job queue (Redis, etc.) — goroutines + PostgreSQL as the state store.
Consolidation jobs are tracked in DB, recoverable on restart.

## Error Handling & Degradation

### Provider Failures
| Failure | Effect | Recovery |
|---------|--------|----------|
| Embedding provider down | Vector search returns 0.0 scores. BM25 + graph still work. Search degrades gracefully. | Retry on next search call. No state lost. |
| LLM provider down | Consolidation and summarization are skipped for this cycle. Observations accumulate uncompressed. Failed jobs marked for retry. | Retry at next scheduled consolidation run. |
| Vision LLM down | N/A — image processing deferred. | |

### Database Failures
| Failure | Effect | Recovery |
|---------|--------|----------|
| Connection lost mid-observe | 500 response. Hook retries (client-side). | pgxpool reconnects automatically. |
| Connection lost mid-consolidation | Job marked as failed. No partial state committed (use DB transaction). | Retry on next consolidation cycle. |
| Migration failure | Server refuses to start. Health check fails. | Manual intervention. |

### General Principles
- **No silent data loss:** Failed operations either complete or leave a clear failure record.
- **Graceful degradation:** If one subsystem is down, others continue serving.
- **Retry with backoff:** Provider calls retry up to 3 times with exponential backoff (1s, 2s, 4s).
- **Health check reflects reality:** `/health` returns 503 if DB is unreachable or migrations are pending.
- **No circuit breaker needed for v2:** Single-instance deployment. Add for multi-node later.

---

## Database Access Rules

- **pgxpool:** connection pool management — acquire, use, release
- **sqlc:** all CRUD and queries — no raw SQL in Go code, ever
- **SQL files:** all queries in `internal/db/queries/*.sql`, compiled to type-safe Go
- **ParadeDB/pgvector/WITH RECURSIVE:** pass through sqlc — sqlc treats unrecognized syntax as opaque SQL, generates correct Go wrappers with zero runtime overhead

---

## Architecture Gaps (Deferred)

These are explicitly NOT in v2 scope:

1. **Pipeline inter-connections** — auto-deriving actions from observations
2. **Crystals/auto timer fallback** — timer-based crystallization trigger
3. **ProceduralMemory consumer** — nothing reads procedural_memories yet

These are architectural gaps that exist in the pipeline design but are deferred
until after the core migration is complete and stable.
