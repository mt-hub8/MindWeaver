# AI Runtime Worker (V8.0 · Ollama)

Python FastAPI worker: **Java → Python Worker → Ollama** (no direct Java→Ollama).

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Config + `ollamaReachable` (does not pull models) |
| POST | `/embed` | Batch embedding via Ollama `/api/embed` |
| POST | `/embeddings` | Legacy Java client format |
| POST | `/generate` | LLM via Ollama `/api/generate` (`stream=false`) |

## Ollama /embed adapter

Worker `/embed` accepts `{ "texts": ["..."], "model": "..." }` (`texts` may also be a single string).

Internally it always calls Ollama with:

```json
{
  "model": "qwen3-embedding:0.6b",
  "input": ["text1", "text2"]
}
```

`input` is **always an array**, even for one text. Ollama responses are read from `embeddings` (fallback: `embedding`).

`GET /health` probes `GET {OLLAMA_BASE_URL}/api/version` only (no model load).

Default `OLLAMA_BASE_URL`: `http://127.0.0.1:11434`

## Environment

| Variable | Default |
|----------|---------|
| `OLLAMA_BASE_URL` | `http://127.0.0.1:11434` |
| `OLLAMA_EMBEDDING_MODEL` | `qwen3-embedding:0.6b` |
| `OLLAMA_LLM_MODEL` | `qwen2.5:7b` |
| `OLLAMA_TIMEOUT_SECONDS` | `120` |

Low-spec LLM fallback: `OLLAMA_LLM_MODEL=qwen2.5:3b`

## Quick Start

```powershell
cd workers/ai-runtime-worker
python -m venv .venv
.\.venv\Scripts\activate
pip install -r requirements.txt
uvicorn main:app --host 0.0.0.0 --port 8001
```

## Java Integration

```powershell
.\mvnw.cmd spring-boot:run "-Dspring-boot.run.profiles=local-ai"
```

See [docs/manual/real-local-ai-runtime-with-ollama.md](../../docs/manual/real-local-ai-runtime-with-ollama.md) for full Windows demo.

## Tests

```powershell
pip install -r requirements.txt
pytest test_worker.py -q
```

Tests mock Ollama; no model download required.

Default Maven tests do **not** start this worker.
