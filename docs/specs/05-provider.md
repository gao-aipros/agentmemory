# v2 Provider Architecture

Finalized 2026-06-21.

## Design Principle

ALL external services go through **langchaingo** — one framework, unified interfaces.
All callers depend only on interfaces, not specific providers.
Provider selection is config-driven via environment variables.
Swap providers without changing application code.

## Embedding

### Interface
`langchaingo embeddings.Embedder` — unified embedding interface.

### Configuration
Four env vars:
- `EMBEDDING_PROVIDER` — provider identifier (openai, voyageai, ollama, etc.)
- `EMBEDDING_API_KEY` — API key for the provider
- `EMBEDDING_MODEL` — model name (e.g., text-embedding-3-small, voyage-3)
- `EMBEDDING_BASE_URL` — optional base URL override (for self-hosted/Ollama/HF TEI)

### Supported Providers

**OpenAI (and compatible):**
- `openai.New()` + `embeddings.NewEmbedder()`
- Compatible services: Ollama, HuggingFace TEI (Text Embeddings Inference)
- Any service exposing an OpenAI-compatible `/v1/embeddings` endpoint works

**Voyage AI:**
- `voyageai.NewVoyageAI()` — directly implements `embeddings.Embedder`
- No OpenAI compatibility layer needed

### Usage
All embedding calls go through `embeddings.Embedder.EmbedDocuments()` or `EmbedQuery()`.
Application code never imports provider-specific packages directly.

---

## LLM

### Interface
`langchaingo llms.Model` — unified LLM interface.

### Configuration
Four env vars:
- `LLM_PROVIDER` — provider identifier (openai, anthropic, deepseek, ollama, etc.)
- `LLM_API_KEY` — API key for the provider
- `LLM_MODEL` — model name (e.g., gpt-4o, claude-sonnet-4-6, deepseek-chat)
- `LLM_BASE_URL` — optional base URL override (for self-hosted/Ollama/vLLM)

### Supported Providers

**OpenAI (and compatible):**
- `openai.New()`
- Compatible services: DeepSeek, Ollama, vLLM, any OpenAI-compatible endpoint
- All go through the same OpenAI client with `LLM_BASE_URL` override

**Anthropic (and compatible):**
- `anthropic.New()` — native Messages API
- Any Anthropic-compatible service

### Usage
All LLM calls go through `llms.Model.GenerateContent()`.
Callers are completely unaware of which provider is active.
The provider is resolved once at startup from env vars and injected.

---

## Consolidation & Embedding Requirements

- **Consolidation requires LLM provider** — summarization and memory extraction need an LLM
- **BM25-only fallback when no LLM provider** — search still works without LLM, but consolidation is disabled
- **Embedding provider required for vector search** — if no embedding provider, vector dimension returns 0 scores; BM25 and graph still work

## Why langchaingo

- Single dependency for ALL external AI services — reduces vendor lock-in
- Interface-based design matches Go idioms — `Embedder` and `Model` are standard Go interfaces
- Provider implementations are thin wrappers — easy to add new providers
- Config-driven selection means operators, not developers, choose providers
- Consistent with v0 pattern (v0 used langchain JS for the same reason)
