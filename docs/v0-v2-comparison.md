# v0 → v2 LLM Context Comparison

Full diff of the LLM context, prompt, and pipeline architecture between
`agentmemory-v1` (TypeScript, v0) and `agentmemory` (Go, v2).

Generated 2026-06-24.

---

## 1. Technology Stack & Provider Architecture

| Dimension | v0 (TypeScript) | v2 (Go) |
|---|---|---|
| Language | TypeScript, Node.js | Go 1.26 |
| LLM abstraction | Hand-rolled `MemoryProvider` interface | `langchaingo llms.Model` |
| Embedding abstraction | Hand-rolled, multi-provider | `langchaingo embeddings.Embedder` |
| Providers | Anthropic, OpenAI, OpenRouter, Gemini, MiniMax, Agent SDK, Noop | `openai-compatible` (OpenAI, DeepSeek, Ollama, vLLM, Groq), `anthropic` |
| Fallback chain | `FallbackChainProvider` with `FALLBACK_PROVIDERS` | None |
| Circuit breaker | `ResilientProvider` wrapper | `RetryWithBackoff` (3 retries, exponential) |
| Active model (dev) | N/A (runs as Claude Code plugin) | `deepseek-v4-flash` via `api.deepseek.com` |
| Embedding model | Multi-provider (OpenAI, Gemini, Voyage, Cohere) | `text-embedding-3-small` via OpenAI |

---

## 2. Context Assembly — Source Buckets

### v0 (dynamic, recency-sorted)

v0's `mem::context` (`src/functions/context.ts`) assembles blocks dynamically:

1. **Pinned slots** — 8 default slots (persona, user_preferences, tool_guidelines,
   project_context, guidance, pending_items, session_patterns, self_notes),
   rendered via `renderPinnedContext()`
2. **Project profile** — auto-learned: top concepts (8), key files (5), conventions,
   common errors (3)
3. **Lessons learned** — ranked by project-match × confidence, capped at 10
4. **Session summaries** — 10 most recent sessions: title, narrative, decisions, files
5. **Raw observations** — fallback when no summary exists; importance ≥ 5, top 5 per session

Blocks are sorted by recency, then greedily filled until the token budget is exhausted.

Separate `mem::enrich` (`src/functions/enrich.ts`) for PreToolUse:
- File context (via `mem::file-context`)
- Relevant observations (via `mem::search`)
- Bug memories (filtered by file match)
- Wrapped in `<agentmemory-relevant-context>` and `<agentmemory-past-errors>` XML tags

Separate `mem::working-context` (`src/functions/working-memory.ts`) for working memory:
- Core Memory (pinned + scored entries, 30% budget)
- Archival Memory (sorted by strength + recency)

### v2 (5 fixed buckets)

v2's `ContextService.AssembleContext()` (`internal/service/context.go`):

| Bucket | Source | Hard Limit |
|---|---|---|
| Observations | `ListObservationsByUserID` (JOIN query) | 20 |
| Recap | `ListSummariesByUserID` (JOIN query) | 3 |
| Lessons | `ListLessonsByTeam` with 30-day confidence decay | 100 queried, filtered |
| Graph | `GraphTraversal` CTE from 10 seed observations | 10 neighbors |
| WorkingMemory | `SlotService.GetSlot("working_memory", "global")` | 300 chars |

Context hooks (`internal/service/context_hooks.go`):
- **SessionStart**: all 5 buckets, 1500-token budget
- **PreToolUse**: search on file paths only, 400-token budget
- **PreCompact**: graph + lessons + working memory only, 600-token budget

### Key Difference

v0 assembles context **dynamically** — any number of blocks, sorted by recency, filled to
budget. v2 has a **fixed 5-bucket structure** with per-source hard limits, then truncated
by priority.

---

## 3. Context Format — XML vs Markdown

### v0: XML outer wrapper + Markdown inner

```xml
<agentmemory-context project="my-project">
## Project Profile
Concepts: auth, database, caching
Key files: src/auth.ts, src/db.ts

## Lessons Learned
- (0.85) Always use parameterized queries — sql_injection
- (0.72) Check cache before querying — performance

## Session abc12345 (2026-06-20)
- [tool_use] Edit src/auth.ts: Added JWT validation
</agentmemory-context>
```

Enrich uses: `<agentmemory-relevant-context>` and `<agentmemory-past-errors>`.

### v2: Pure Markdown

```markdown
### Context (AgentMemory v2)
Date: 2026-06-24

### Memory Graph
[obs_abc] 2026-06-20: Added JWT validation (graph_score: 0.85)

### Relevant Lessons
[lesson_def] (confidence: 0.85): Always use parameterized queries

### Relevant Observations
[obs_ghi] 2026-06-23 write_file: Updated migration script

### Session Recap
[session jkl] 2026-06-23: Worked on auth flow, fixed 3 bugs

### Working Memory
[wm] Currently debugging OAuth token refresh
```

### Why XML wrapping matters

1. **Boundary clarity**: The model can cleanly distinguish injected context from user
   conversation. `<agentmemory-context>` creates an unambiguous "this section is different"
   signal.
2. **Prompt injection defense**: If memory content contains text like "ignore previous
   instructions and do X," markdown alone has no mechanism to scope its influence. XML
   tags create clear containment.
3. **Anthropic's recommendation**: Anthropic's documentation specifically recommends XML
   tags for structured context — Claude models handle them most reliably.

---

## 4. Token Budgets & Limits

| Dimension | v0 | v2 |
|---|---|---|
| Default budget | 2000 tokens (`TOKEN_BUDGET`) | 1500 tokens |
| Source/overhead split | None — single budget | 1100 source + 400 overhead |
| Token estimation | `ceil(len / 3)` (3 chars/token) | `len / 4` (4 chars/token) |
| Truncation strategy | Greedy skip: newest-first, block-level | Priority-based: recap → observations → lessons → graph (trimmed in that order) |
| Working memory | Participates in budget alongside other blocks | **Never truncated** — always preserved |
| Per-hook budgets | Single budget for all hooks | Differentiated: SessionStart=1500, PreToolUse=400, PreCompact=600 |
| Bucket allocation weights | None — dynamic fill to budget | Graph 20%, Lessons 20%, Observations 25%, Recap 15%, WM 20% |

---

## 5. LLM Prompt Templates & Output Format

| Dimension | v0 | v2 |
|---|---|---|
| Output format | **XML** — all prompts enforce tags like `<observation>`, `<summary>`, `<facts>`, `<insights>`, `<entities>` | **JSON** — all prompts ask for JSON arrays/objects |
| System prompts | Separate system prompt per function (7 total: `COMPRESSION_SYSTEM`, `SUMMARY_SYSTEM`, `REDUCE_SYSTEM`, `SEMANTIC_MERGE_SYSTEM`, `PROCEDURAL_EXTRACTION_SYSTEM`, `REFLECT_SYSTEM`, `GRAPH_EXTRACTION_SYSTEM`) | Instructions baked into user message, no separate system prompt |
| Validation | Zod schemas + `compressWithRetry()` (re-prompts LLM on parse failure) | `json.Unmarshal` — no retry on parse failure |
| Quality scoring | `scoreCompression()` / `scoreSummary()` heuristic 0-100 | None |
| Synthetic fallback | `buildSyntheticCompression()` — zero-LLM heuristics (type inference, file extraction, 0.3 confidence) | None |
| Chunking | 400 obs/chunk, parallel processing (max concurrency 6), 50% failure threshold, `REDUCE_SYSTEM` merge step | `ChunkObservations()` — 3000-token budget, simpler |

### v0 System Prompts (7 total)

| Prompt | File | Purpose |
|---|---|---|
| `COMPRESSION_SYSTEM` | `src/prompts/compression.ts` | Compress tool-use observation → `<observation>` XML |
| `SUMMARY_SYSTEM` | `src/prompts/summary.ts` | Summarize session observations → `<summary>` XML |
| `REDUCE_SYSTEM` | `src/prompts/summary.ts` | Merge chunked partial summaries → single `<summary>` |
| `SEMANTIC_MERGE_SYSTEM` | `src/prompts/consolidation.ts` | Merge episodic memories → `<facts>` XML |
| `PROCEDURAL_EXTRACTION_SYSTEM` | `src/prompts/consolidation.ts` | Extract procedures from patterns → `<procedures>` XML |
| `REFLECT_SYSTEM` | `src/prompts/reflect.ts` | Synthesize cross-cutting insights → `<insights>` XML |
| `GRAPH_EXTRACTION_SYSTEM` | `src/prompts/graph-extraction.ts` | Extract entities + relationships → `<entities>` + `<relationships>` XML |

### v2 Prompt Functions (4 total)

| Function | File | Output |
|---|---|---|
| `BuildCompressionPrompt` / `BuildBatchCompressionPrompt` | `internal/service/compress_llm.go` | JSON array of `CompressionResult` |
| `BuildSummarizePrompt` / `BuildIncrementalSummarizePrompt` | `internal/service/summarize.go` | Plain text summary |
| `BuildConsolidationPrompt` | `internal/service/consolidate.go` | JSON `{memories, lessons}` |
| *(reflect — not yet implemented)* | — | — |

---

## 6. Pipeline Stages

| Stage | v0 | v2 |
|---|---|---|
| **Observe** | Hooks capture → KV store | Hooks capture → PostgreSQL via `ObservationService` |
| **Compress** | `mem::compress` (LLM) or synthetic (zero-LLM); per-observation, retry on parse failure | `compress_llm.go` — batch or single, JSON output, no retry |
| **Summarize** | `mem::summarize` — chunks large sessions, parallel chunk processing, reduce step | `summarize.go` — chunks by token budget, incremental update mode |
| **Consolidate** | `mem::consolidate-pipeline`: semantic merge (episodic→facts), procedural extraction (patterns→procedures), reflect, decay | `consolidate.go`: single `ConsolidationResult` with `memories` + `lessons` arrays |
| **Reflect** | `mem::reflect` — concept clusters (BFS or Jaccard), per-cluster LLM call → `<insights>` | **Not implemented** — scheduler has Tier 3 slot, code not written |
| **Graph extract** | `mem::graph-extract` — LLM extracts entities + relationships from observations | Database CTE (`GraphTraversal`) — no LLM, purely structural |
| **Crystallize** | `mem::crystallize` — LLM synthesis of action chains → narrative + outcomes + lessons | `crystallize.go` — MVP stub, no LLM ("pending in future release") |

---

## 7. Context Injection Triggers

| Trigger | v0 | v2 |
|---|---|---|
| SessionStart | Full `mem::context` (all blocks, dynamic fill) | Full `AssembleContext()` (5 buckets, 1500t) |
| PreToolUse | `mem::enrich` — file context + search + bug memories, 4000-char cap | Search on file paths, 400-token budget |
| PreCompact | Condensed context refresh | Graph + lessons + WM only, 600-token budget |
| UserPromptSubmit | Captures prompt as observation | ❌ Not present |
| PostToolUse | Captures tool output (with image extraction) | ❌ Not present |
| SubagentStart/Stop | Captured | ❌ Not present |
| Gate | `AGENTMEMORY_INJECT_CONTEXT=true` (default: off) | `AGENTMEMORY_INJECT_CONTEXT` (default: off) |

v0 registers **10 hook types**; v2 has only 3 context injection hooks.

---

## 8. Features Present in v0, Absent from v2

1. **Reflect stage** — higher-order insight synthesis via LLM (scheduler slot exists, code missing)
2. **LLM-based graph extraction** — entity + relationship extraction from observations
3. **Project profiles** — auto-learned concepts, key files, conventions, common errors per project
4. **Claude bridge** — auto-syncs project summaries + key memories to `MEMORY.md`
5. **Procedural extraction** — extracting reusable procedures from recurring patterns
6. **Bug memory matching** — enriches PreToolUse with file-matched bug memories
7. **Quality scoring** — heuristic scoring of LLM compression/summary output (0-100)
8. **Synthetic (zero-LLM) compression** — heuristics-only fallback with confidence tagging
9. **Fallback provider chain** — cascading provider failover
10. **Circuit breaker** — `ResilientProvider` wrapper
11. **Agent scope isolation** — `AGENT_ID` + `AGENTMEMORY_AGENT_SCOPE` for multi-agent setups
12. **Premium model cost warning** — warns about expensive OpenRouter models
13. **Snapshot-based persistence** — for standalone mode
14. **XML-wrapped context** — `<agentmemory-context project="...">` outer wrapper
15. **Retry on LLM parse failure** — re-prompts with stricter instructions
16. **Smart search follow-up rate diagnostics** — `AGENTMEMORY_FOLLOWUP_WINDOW_SECONDS`
17. **Image/vision description** — `VISION_DESCRIPTION_PROMPT` for multi-modal

---

## 9. Features Present in v2, Absent from v0

1. **Weighted bucket allocation** — 20/20/25/15/20% with explicit truncation priority order
2. **Differentiated hook budgets** — SessionStart=1500, PreToolUse=400, PreCompact=600
3. **Working memory truncation protection** — never truncated, always preserved
4. **Team-scoped lessons** — lessons scoped to teams, not just projects
5. **Confidence decay filter** — lessons older than 30 days with confidence < 0.3 excluded
6. **Graph traversal via CTE** — database-driven neighbors without LLM cost
7. **langchaingo abstraction** — provider-agnostic via standard Go library
8. **PostgreSQL-backed** — relational DB with JOINs, not KV store
9. **sqlc type-safe queries** — zero raw SQL in application code
10. **4-tier goroutine scheduler** — no external queue dependency

---

## 10. Storage Architecture

| Dimension | v0 | v2 |
|---|---|---|
| Primary store | iii-engine KV store (in-memory + file snapshots) | PostgreSQL via ParadeDB |
| Search index | BM25 + vector via iii-engine | BM25 (pg_search) + vector (pgvector HNSW) |
| Graph | In-memory, snapshot-based | PostgreSQL `WITH RECURSIVE` CTE |
| Schema | Dynamic (KV namespaces) | Static (25 tables, 42 indexes) |
| Session state | KV namespaces (`KV.sessions`, `KV.observations()`) | Relational tables (`sessions`, `observations`) |
