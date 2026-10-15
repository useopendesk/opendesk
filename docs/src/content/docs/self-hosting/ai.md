---
title: AI
description: Turn on opendesk's AI features with Anthropic, OpenAI or a local model, and set up semantic search.
---

AI features are off until you configure a provider. Self-hosted instances have no AI balance or usage limits: you pay your provider directly, or nothing at all with a local model. Workspace admins can still turn AI off for their workspace. See [AI features](/guides/ai/).

## Anthropic

```sh
ANTHROPIC_API_KEY=sk-ant-...
```

With only this key set, the provider is detected as `anthropic`, and the models default to `claude-sonnet-5-5` (`AI_MODEL`, for drafts, summaries and answers) and `claude-haiku-4-5-20251001` (`AI_FAST_MODEL`, for triage, classification and short rewrites).

## OpenAI and compatible endpoints

`AI_PROVIDER=openai` works with any OpenAI-compatible API: OpenAI, Azure OpenAI, Ollama, vLLM or LM Studio. Set the model names your endpoint serves.

```sh
# OpenAI
OPENAI_API_KEY=sk-...
AI_MODEL=<model>
AI_FAST_MODEL=<smaller model>
```

### Local models with Ollama

The self-host bundle has an `ollama` compose profile that runs Ollama next to opendesk and pulls the models for you: run the installer with `--ollama`, or see [Docker Compose profiles](/self-hosting/docker-compose/#ollama). To use an Ollama you already run:

Run everything on your own hardware, so customer content never leaves your network:

```sh
AI_PROVIDER=openai
OPENAI_BASE_URL=http://ollama:11434/v1
OPENAI_API_KEY=ollama            # any non-empty value
AI_MODEL=<a model you have pulled>
```

### vLLM

```sh
AI_PROVIDER=openai
OPENAI_BASE_URL=http://vllm:8000/v1
OPENAI_API_KEY=<your vLLM api key, or any value>
AI_MODEL=<the served model name>
```

When `AI_FAST_MODEL` is empty, `AI_MODEL` is used for everything.

## Embeddings and semantic search

Embeddings power semantic search and the chat answer bot's article retrieval.

| `EMBEDDINGS_DRIVER` | When |
| --- | --- |
| `voyage` | Chosen automatically when `VOYAGE_API_KEY` is set. |
| `openai` | Chosen automatically with the `openai` provider. Uses the same endpoint. |
| `none` | The default otherwise. Search falls back to Postgres full-text search. |

Set `EMBEDDINGS_MODEL` to choose the model. Changing the embeddings model reindexes existing content. Published articles are indexed when they are published, and tickets when they are solved.

### pgvector

Embeddings are stored in Postgres and need the `vector` extension (pgvector). Managed services such as PlanetScale and Neon have it, and so does the `pgvector/pgvector:pg17` image. On a Postgres without it, opendesk keeps working and semantic search uses full-text search instead.

## Decisions with Jev

Triage, the chat answer bot's relevance check and `ai_decide` automation conditions are typed decisions: a choice, a score on ordered levels, or the probability of yes. Ollie makes them using Jev, TypeSafe's decision model, when a TypeSafe key is set. Jev returns calibrated probabilities and a confidence for each answer, so triage applies only what it is sure of and leaves the rest as suggestions.

```sh
TYPESAFE_API_KEY=...
# optional
TYPESAFE_MODEL=jev-latest          # or a pinned version such as jev-1.13.0
```

Through the Vercel AI Gateway:

```sh
TYPESAFE_API_KEY=<AI Gateway key>
TYPESAFE_API_URL=https://ai-gateway.vercel.sh/typesafe
TYPESAFE_MODEL=typesafe-ai/jev
```

`DECISIONS_PROVIDER` picks the engine: `typesafe`, `llm` or `none`. When empty it is `typesafe` with a TypeSafe key, else `llm` when an AI provider is configured, else `none`. On `llm`, the AI provider's fast model answers the same questions as structured output, without a calibrated confidence. Without a TypeSafe key every feature keeps working; nothing depends on Jev. Triage also runs with Jev alone, without an AI provider.

## Privacy

- Customer content is sent only to the providers you configure (the AI provider, and TypeSafe when its key is set).
- Each request carries only the data that feature needs.
- Prompts and completions are never logged. Usage records store counts only.

## Check it works

After restarting opendesk, `GET /v1/ai/config` reports `enabled: true` with the provider and models. See the [AI API](/api/resources/ai/).
