# v2 Provider Architecture

## Embedding

- **Interface:** langchaingo `embeddings.Embedder` unified interface, config-driven
- **Env vars:** `EMBEDDING_PROVIDER` / `EMBEDDING_API_KEY` / `EMBEDDING_MODEL` / `EMBEDDING_BASE_URL`
- **OpenAI (and compat):** `openai.New()` + `embeddings.NewEmbedder()` — Ollama, HF TEI
- **Voyage:** `voyageai.NewVoyageAI()`
- Rest of code depends only on the interface

## LLM

- **Interface:** langchaingo `llms.Model` unified interface, config-driven
- **Env vars:** `LLM_PROVIDER` / `LLM_API_KEY` / `LLM_MODEL` / `LLM_BASE_URL`
- **OpenAI (and compat):** `openai.New()` — DeepSeek, Ollama, vLLM
- **Anthropic (and compat):** `anthropic.New()` — native Messages API
- Callers only use `GenerateContent()`, unaware of provider

## Design Principle

All callers depend only on interfaces, not specific providers.
Providers are swappable via configuration.
