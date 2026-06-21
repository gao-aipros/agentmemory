# v2 Pipeline Architecture

Preserved from v0.

## Pipeline Stages

```
observe → compress → summarize → consolidate → reflect
```

- **Dual Observation/Action pipelines** carry over from v0
- **Dual Context/Search consumption lines** carry over from v0

## Data Flow

| Stage | Output | Destination |
|-------|--------|-------------|
| observe | raw observation | session store |
| compress | CompressedObservation | BM25 + Vector dual index |
| summarize | SessionSummary | Context injection only (NOT search index) |
| consolidate | SemanticMemory (facts) | reflect + retention (NOT search index) |
| consolidate | Memory (pattern) → ProceduralMemory | procedural store |
| consolidate | Graph → Insights | insights store |
| reflect | higher-order insights | insights table |

## Context Injection

- **Hard limit:** 1500 tokens
- **Format:** Each item = one-line summary + recall ID (reference format)
- **5 source buckets** (see 06-team-user.md for budget table)

## Implementation Rule: v0 as Living Spec

- v0 is v2's **behavioral spec**, not a code template
- When unclear: read v0 source for behavior — what functions do, how pipeline connects, how data flows
- v0 source: https://github.com/Noodle05/agentmemory
- Reference behavior semantics, don't copy code (TS → Go, entirely different implementation)

## Deferred Gaps

- Pipeline inter-connections (observation → action auto-derivation)
- Crystals/auto timer fallback
- ProceduralMemory consumer
