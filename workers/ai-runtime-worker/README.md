# AI Runtime Worker (V6.0)

Python FastAPI worker providing **embedding** and **LLM generation** runtime for the Java orchestrator.

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| POST | `/embed` | Batch embedding (`texts`, `model`) |
| POST | `/embeddings` | OpenAI-compatible embedding (legacy Java client) |
| POST | `/generate` | LLM generation (`systemPrompt`, `userPrompt`, `model`, …) |

## Quick Start

```bash
cd workers/ai-runtime-worker
python -m venv .venv
.venv\Scripts\activate   # Windows
pip install -r requirements.txt
uvicorn main:app --host 127.0.0.1 --port 8001
```

## First Run / Model Download

- Embedding model default: `sentence-transformers/all-MiniLM-L6-v2` (384 dims)
- First request triggers Hugging Face download; requires network once
- CPU-only; suitable for local dev, not production SLA

## LLM Note

`/generate` uses a **deterministic template** for local dev (no GPU LLM required).
Swap in a real model backend later without changing Java API contracts.

## Java Integration

Start Spring Boot with profile `local-ai`:

```bash
.\mvnw.cmd spring-boot:run -Dspring-boot.run.profiles=local-ai
```

Properties: `application-local-ai.properties`

- `app.embedding.provider=local-worker` → `http://127.0.0.1:8001/embeddings`
- `app.llm.provider=local-python` → `http://127.0.0.1:8001/generate`

Default tests use **mock** providers and do **not** start this worker.
