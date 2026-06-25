# v0 → v2 Migration Recommendations

What to migrate from `agentmemory-v1` (TypeScript) to `agentmemory` (Go),
ordered by priority and impact.

Generated 2026-06-24. Based on full comparison in `v0-v2-comparison.md`.

---

## Tier 1 — Critical Gaps (v2 spec requires but not yet implemented)

### 1. XML Context Wrapper

**Effort**: Small | **Impact**: High | **Risk**: Adds prompt injection defense

v0 wraps all injected context in XML (`<agentmemory-context project="...">`).
v2 uses pure Markdown headers (`### Context (AgentMemory v2)`).

**Why it matters**:
- XML tags create unambiguous boundaries the model reliably recognizes
- Prevents prompt injection: if memory content says "ignore previous instructions,"
  markdown has no scoping mechanism; XML containment limits its blast radius
- Anthropic's documentation specifically recommends XML for structured context

**What to do**:
- Wrap `ApplyBudget()` output in `<agentmemory-context version="2">...</agentmemory-context>`
- Keep the internal Markdown structure (it works fine for readability)
- Add `project` attribute to the wrapper (like v0 does)
- This is v0's proven pattern: XML outer boundary, Markdown inner structure

### 2. Synthetic Compression Fallback

**Effort**: Small | **Impact**: Medium | **Risk**: Prevents silent data loss

v0 has `buildSyntheticCompression()` — when LLM is disabled or fails, it infers
type from tool name, extracts file paths, and creates a basic narrative with 0.3
confidence. v2 has no fallback at all.

**Why it matters**:
- In zero-LLM mode, observations still get indexed for BM25 search
- If one LLM call fails, the observation isn't lost — it gets a degraded but usable entry
- This is the difference between "degraded search quality" and "no search at all"

**What to do**:
- Add `buildSyntheticCompression()` in `internal/service/compress_synthetic.go`
- Infer `type` from tool name heuristics, extract file paths, generate basic narrative
- Tag with `confidence: 0.3` and `method: "synthetic"` so downstream knows it's low-quality
- Call as fallback when LLM compress returns error or in no-LLM mode

### 3. Reflect Stage Implementation

**Effort**: Large | **Impact**: High | **Risk**: Completes the pipeline spec

v2's scheduler has Tier 3 (`processReflection`) but the implementation is missing.
The pipeline spec (`docs/specs/07-pipeline.md`) explicitly requires the full
`observe → compress → summarize → consolidate → reflect` pipeline.

v0's `mem::reflect`:
- Builds concept clusters from graph nodes (BFS traversal) or Jaccard similarity fallback
- Calls LLM with `REFLECT_SYSTEM` prompt per cluster
- Parses `<insight confidence="..." title="...">` XML output
- Creates or reinforces Insight entries with confidence tracking

**Why it matters**:
- This is the highest-level reasoning tier — finds cross-cutting patterns invisible
  to single-observation analysis
- Without reflect, the pipeline stops at "extract facts from sessions" and never
  reaches "synthesize higher-order understanding"
- The scheduler slot and spec already exist — this is completing agreed-upon work

**What to do**:
- Implement `internal/service/reflect.go` with `ReflectionService`
- Port `REFLECT_SYSTEM` prompt from v0 (adapt XML output to JSON or keep XML)
- Wire into scheduler Tier 3 via `HasUnreflectedMemories()` query
- Add `insights` table + sqlc queries if not present

---

## Tier 2 — High-Value Features

### 4. LLM-Based Graph Extraction

**Effort**: Medium | **Impact**: Medium | **Risk**: Complements existing CTE traversal

v2 uses database CTE (`GraphTraversal`) for graph neighbors — fast, zero-LLM, but
can only traverse edges that already exist. v0's `GRAPH_EXTRACTION_SYSTEM` uses an
LLM to discover **new** entities and relationships from observation content.

**Why it matters**:
- CTE traversal is structural (edge exists → follow it); LLM extraction is semantic
  (these two things are related even though no edge was explicitly created)
- The two approaches complement each other: CTE for fast context injection,
  LLM extraction (async, post-compression) for enriching the graph structure
- Without LLM extraction, the graph only grows when code explicitly creates edges

**What to do**:
- Add `internal/service/graph_extract.go` with `GraphExtractionService`
- Port `GRAPH_EXTRACTION_SYSTEM` prompt, adapt to JSON output
- Run as async step after compression in the pipeline
- Store extracted entities in `graph_nodes`, relationships in `graph_edges`
- Keep CTE traversal as the read path; LLM extraction as the write/enrich path

### 5. Project Profiles (Auto-Learned)

**Effort**: Medium | **Impact**: High | **Risk**: Best ROI for context relevance

v0's `ProjectProfile` learns over time: top concepts, key files, conventions, and
common errors — all derived from observation patterns. Injected into every context.

**Why it matters**:
- This is the single highest-ROI feature for cross-session context relevance
- A new session on project X immediately gets: "Concepts: auth, oauth2, jwt;
  Key files: src/auth.ts, src/middleware.ts; Conventions: always check token expiry first"
- Without this, context injection only has per-session granularity — no project-level
  "these are the things you need to know about this codebase"

**What to do**:
- Add `project_profiles` table: project_id, top_concepts (jsonb), top_files (jsonb),
  conventions (text[]), common_errors (text[])
- Implement `ProfileService.UpdateProfile()` — called after consolidation, updates
  concept frequencies and file references
- Add profile section to `ApplyBudget()` output (under XML wrapper)
- Port v0's frequency-tracking logic

### 6. Claude Bridge (MEMORY.md Sync)

**Effort**: Medium | **Impact**: High | **Risk**: Works even when server is down

v0 writes project summaries and key memories to `~/.claude/projects/<slug>/MEMORY.md`.
This means Claude Code sees persistent memory in **every session**, even without the
agentmemory server running.

**Why it matters**:
- Context injection only works when the server is running AND `AGENTMEMORY_INJECT_CONTEXT=true`
- MEMORY.md is Claude Code's native persistence — it reads it at every session start regardless
- This is a second distribution channel: server-driven context injection for active sessions,
  file-based persistence for all sessions
- Survives server restarts, migrations, and offline work

**What to do**:
- Add `internal/service/claude_bridge.go` with `ClaudeBridgeService`
- Port v0's slug computation: `/<absolute>/path` → `-absolute-path`
- Write `## Agent Memory (auto-synced by agentmemory)` section with
  `## Project Summary` and `## Key Memories` subsections
- Respect `CLAUDE_MEMORY_LINE_BUDGET` (default 200 lines)
- Trigger on consolidate completion

### 7. PreToolUse Enrichment (Bug Memory Matching)

**Effort**: Small | **Impact**: Medium | **Risk**: Prevents repeating known bugs

v0's `mem::enrich` for PreToolUse does three things: file context, relevant observations,
and **bug memory matching** (finds bugs related to files being edited). v2's
`TriggerPreToolUse` only does a search on file paths.

**Why it matters**:
- Before editing `auth.ts`, the agent sees: "Past errors in this file: JWT expiry
  not checked before parsing (3 occurrences)"
- This is a safety net — the agent learns from past mistakes without explicitly
  querying for them
- Implementation is lightweight: filter bug-type memories by file match, cap at 3

**What to do**:
- Extend `TriggerPreToolUse` in `internal/service/context_hooks.go`
- Add bug memory lookup: filter memories where `type == "bug"` and files overlap
- Wrap in `<agentmemory-past-errors>` XML tag like v0
- Cap at 3 bug memories, sorted by recency

---

## Tier 3 — Quality & Robustness

### 8. LLM Output Quality Scoring

**Effort**: Small | **Impact**: Medium | **Risk**: Early detection of LLM degradation

v0's `scoreCompression()` and `scoreSummary()` provide 0-100 heuristic scores based
on field presence, length, and structure. v2 does no quality validation beyond
JSON parse success.

**What to do**:
- Add `ScoreCompression(result) float64` and `ScoreSummary(text) float64`
- Score dimensions: required fields present, non-empty, reasonable length
- Log warnings when score drops below threshold
- Feed score into retry decision (see #9)

### 9. Retry on LLM Parse Failure

**Effort**: Small | **Impact**: Medium | **Risk**: Reduces silent data loss

v0's `compressWithRetry()` re-prompts the LLM with a `STRICTER_SUFFIX` when
validation fails. v2 does a single `json.Unmarshal` and returns the error.

**What to do**:
- Wrap `ParseCompressionResponse` / `ParseBatchCompressionResponse` in retry logic
- On parse failure, append stricter formatting instruction and retry (max 2 attempts)
- Combine with quality scoring: retry if score < 30

### 10. Fallback Provider Chain

**Effort**: Medium | **Impact**: Low-Medium | **Risk**: Production reliability

v0's `FallbackChainProvider` tries providers sequentially. If DeepSeek is down,
it falls through to OpenAI → Anthropic → OpenRouter.

**What to do**:
- Add `FallbackLLMService` wrapping multiple `llms.Model` instances
- Configure via `LLM_FALLBACK_PROVIDERS` env var
- Try each in order, return first success
- Log fallback events clearly

---

## Tier 4 — Deferred (Do Not Migrate Now)

These are explicitly deferred per `docs/specs/07-pipeline.md`:

| Feature | Reason |
|---|---|
| ProceduralMemory consumer | Nothing reads procedural_memories yet — spec defers until after core migration |
| Crystallize (LLM-driven) | v2 stub exists, marked "LLM summarization pending in future release" |
| Image/vision description | Multi-modal not in v2 scope |
| Smart search follow-up diagnostics | Nice-to-have, not essential |
| Agent scope isolation | Only needed for multi-agent setups |
| Premium model cost warning | v2 currently targets single-provider deployment |

---

## Recommended Implementation Order

```
1. XML context wrapper        ← smallest change, biggest defense win
2. Synthetic compression      ← prevents silent data loss
3. Retry + quality scoring    ← foundational robustness for all LLM calls
4. Reflect stage              ← completes the pipeline per spec
5. LLM graph extraction       ← enriches graph beyond CTE
6. PreToolUse bug matching    ← lightweight safety net
7. Project profiles           ← highest context-ROI feature
8. Claude bridge              ← second distribution channel
9. Fallback provider chain    ← production hardening
```

### Rationale for ordering

- **1-3** are small, self-contained changes that harden the existing system before adding
  new features. Each can be done in a single session.
- **4** is the biggest missing pipeline stage — it completes the architectural promise.
- **5** complements the existing CTE traversal without replacing it.
- **6** is a 50-line addition to existing PreToolUse code.
- **7-8** are the richest features but require new tables, services, and deeper design.
- **9** is infrastructure hardening that matters most in production.
