"""AI Runtime Worker – Ollama adapter (V8.0)."""

from __future__ import annotations

import json
import os
import time
from datetime import datetime, timezone
from typing import Any, List, Optional, Tuple, Union

import httpx
from fastapi import FastAPI, HTTPException
from fastapi.responses import JSONResponse
from pydantic import BaseModel, Field, field_validator

PROVIDER = "local-ollama"

OLLAMA_BASE_URL = os.getenv("OLLAMA_BASE_URL", "http://127.0.0.1:11434").rstrip("/")
OLLAMA_EMBEDDING_MODEL = os.getenv("OLLAMA_EMBEDDING_MODEL", "qwen3-embedding:0.6b")
OLLAMA_LLM_MODEL = os.getenv("OLLAMA_LLM_MODEL", "qwen2.5:7b")
OLLAMA_TIMEOUT_SECONDS = float(os.getenv("OLLAMA_TIMEOUT_SECONDS", "120"))
OLLAMA_HEALTH_TIMEOUT_SECONDS = 3.0

app = FastAPI(title="AI Task Orchestrator AI Runtime Worker (Ollama)")


class UTF8JSONResponse(JSONResponse):
    """Return JSON as UTF-8 bytes with an explicit charset for clients."""

    media_type = "application/json; charset=utf-8"

    def render(self, content: Any) -> bytes:
        return json.dumps(
            content,
            ensure_ascii=False,
            separators=(",", ":"),
        ).encode("utf-8")


class OllamaClientError(Exception):
    def __init__(
        self,
        message: str,
        code: str = "OLLAMA_ERROR",
        status_code: int = 502,
        detail: Optional[Any] = None,
    ) -> None:
        super().__init__(message)
        self.message = message
        self.code = code
        self.status_code = status_code
        self.detail = detail if detail is not None else message


def ollama_embed_url() -> str:
    return f"{OLLAMA_BASE_URL}/api/embed"


def ollama_tags_url() -> str:
    return f"{OLLAMA_BASE_URL}/api/tags"


def check_ollama_reachable() -> Tuple[bool, Optional[str]]:
    tags_url = ollama_tags_url()
    try:
        with httpx.Client(timeout=OLLAMA_HEALTH_TIMEOUT_SECONDS, trust_env=False) as client:
            response = client.get(tags_url)
        if 200 <= response.status_code < 300:
            return True, None
        return False, f"HTTPStatusError: HTTP {response.status_code}"
    except Exception as exc:
        return False, f"{type(exc).__name__}: {exc}"


def is_ollama_reachable() -> bool:
    reachable, _error = check_ollama_reachable()
    return reachable


def embed_texts(texts: List[str], model: Optional[str] = None) -> Tuple[List[List[float]], str, int, int]:
    if not texts:
        raise OllamaClientError("texts must not be empty", code="INVALID_REQUEST", status_code=400)

    model_name = (model or OLLAMA_EMBEDDING_MODEL).strip()
    if not model_name:
        raise OllamaClientError("model must not be blank", code="INVALID_REQUEST", status_code=400)

    started = time.perf_counter()
    vectors: List[List[float]] = []
    resolved_model = model_name

    try:
        with httpx.Client(timeout=OLLAMA_TIMEOUT_SECONDS, trust_env=False) as client:
            for text in texts:
                vector, resolved_model = embed_single_text(client, text, model_name)
                vectors.append(vector)
    except OllamaClientError:
        raise
    except httpx.TimeoutException as exc:
        raise OllamaClientError(
            f"Ollama embed timeout after {OLLAMA_TIMEOUT_SECONDS}s",
            code="OLLAMA_TIMEOUT",
            status_code=504,
        ) from exc
    except httpx.RequestError as exc:
        raise OllamaClientError(
            f"Ollama unavailable: {exc}",
            code="OLLAMA_UNAVAILABLE",
            status_code=503,
            detail=str(exc),
        ) from exc

    latency_ms = int((time.perf_counter() - started) * 1000)
    dimension = len(vectors[0])
    return vectors, resolved_model, dimension, latency_ms


def embed_single_text(
    client: httpx.Client,
    text: str,
    model_name: str,
) -> Tuple[List[float], str]:
    ollama_url = ollama_embed_url()
    payload = {
        "model": model_name,
        "input": text,
    }

    response = client.post(ollama_url, json=payload)

    if response.status_code >= 400:
        raise OllamaClientError(
            f"Ollama embed HTTP {response.status_code}",
            code="OLLAMA_BAD_RESPONSE",
            status_code=502,
            detail={
                "ollamaUrl": ollama_url,
                "model": model_name,
                "inputLength": len(text),
                "statusCode": response.status_code,
                "responseBody": response.text,
            },
        )

    try:
        body = response.json()
    except ValueError as exc:
        raise OllamaClientError(
            "Ollama embed returned non-JSON response",
            code="OLLAMA_BAD_RESPONSE",
            status_code=502,
            detail={
                "ollamaUrl": ollama_url,
                "model": model_name,
                "inputLength": len(text),
                "statusCode": response.status_code,
                "responseBody": response.text,
            },
        ) from exc

    vector = extract_single_embedding(body)
    resolved_model = str(body.get("model") or model_name)
    return vector, resolved_model


def extract_single_embedding(body: dict[str, Any]) -> List[float]:
    if isinstance(body.get("embeddings"), list) and body["embeddings"]:
        vector = normalize_vector(body["embeddings"][0])
    elif isinstance(body.get("embedding"), list):
        vector = normalize_vector(body["embedding"])
    else:
        raise OllamaClientError(
            "Ollama embed response missing embeddings/embedding field",
            code="OLLAMA_BAD_RESPONSE",
            status_code=502,
            detail={
                "responseBody": str(body),
            },
        )

    if not vector:
        raise OllamaClientError(
            "Ollama embed returned an empty embedding vector",
            code="OLLAMA_BAD_RESPONSE",
            status_code=502,
        )
    return vector


def decode_response_body(response: httpx.Response, errors: str = "replace") -> str:
    return response.content.decode("utf-8", errors=errors)


def parse_generate_response_json(response: httpx.Response) -> dict[str, Any]:
    # Parse raw response bytes only. Never use response.text: charset detection can
    # pick latin-1/cp1252 on Windows and corrupt Chinese in the JSON "response" field.
    raw = response.content
    if not raw:
        raise OllamaClientError(
            "Ollama generate returned empty body",
            code="OLLAMA_BAD_RESPONSE",
            status_code=502,
        )
    try:
        return json.loads(raw)
    except json.JSONDecodeError:
        try:
            return json.loads(raw.decode("utf-8"))
        except UnicodeDecodeError as exc:
            raise OllamaClientError(
                "Ollama generate returned non-UTF-8 response",
                code="OLLAMA_BAD_RESPONSE",
                status_code=502,
                detail=decode_response_body(response),
            ) from exc
        except json.JSONDecodeError as exc:
            raise OllamaClientError(
                "Ollama generate returned non-JSON response",
                code="OLLAMA_BAD_RESPONSE",
                status_code=502,
                detail=decode_response_body(response),
            ) from exc


def extract_generate_content(body: dict[str, Any]) -> str:
    content = body.get("response", "")
    if not isinstance(content, str):
        if isinstance(content, bytes):
            content = content.decode("utf-8")
        else:
            raise OllamaClientError(
                "Ollama generate response field is not text",
                code="OLLAMA_BAD_RESPONSE",
                status_code=502,
                detail=str(type(content)),
            )
    if not content.strip():
        raise OllamaClientError(
            "Ollama generate returned empty content",
            code="OLLAMA_BAD_RESPONSE",
            status_code=502,
            detail=str(body),
        )
    return content


def generate_text(
    system_prompt: Optional[str],
    user_prompt: str,
    model: Optional[str] = None,
    temperature: float = 0.2,
    max_tokens: int = 1200,
) -> Tuple[str, str, Optional[dict[str, int]], int, str]:
    user_prompt = (user_prompt or "").strip()
    if not user_prompt:
        raise OllamaClientError("userPrompt must not be blank", code="INVALID_REQUEST", status_code=400)

    model_name = (model or OLLAMA_LLM_MODEL).strip()
    if not model_name:
        raise OllamaClientError("model must not be blank", code="INVALID_REQUEST", status_code=400)

    prompt_parts: List[str] = []
    if system_prompt and system_prompt.strip():
        prompt_parts.append(system_prompt.strip())
    prompt_parts.append(user_prompt)
    prompt = "\n\n".join(prompt_parts)

    payload = {
        "model": model_name,
        "prompt": prompt,
        "stream": False,
        "options": {
            "temperature": temperature,
            "num_predict": max_tokens,
        },
    }

    started = time.perf_counter()
    try:
        with httpx.Client(timeout=OLLAMA_TIMEOUT_SECONDS, trust_env=False) as client:
            response = client.post(f"{OLLAMA_BASE_URL}/api/generate", json=payload)
    except httpx.TimeoutException as exc:
        raise OllamaClientError(
            f"Ollama generate timeout after {OLLAMA_TIMEOUT_SECONDS}s",
            code="OLLAMA_TIMEOUT",
            status_code=504,
        ) from exc
    except httpx.RequestError as exc:
        raise OllamaClientError(
            f"Ollama unavailable: {exc}",
            code="OLLAMA_UNAVAILABLE",
            status_code=503,
            detail=str(exc),
        ) from exc

    latency_ms = int((time.perf_counter() - started) * 1000)

    if response.status_code >= 400:
        raise OllamaClientError(
            f"Ollama generate HTTP {response.status_code}",
            code="OLLAMA_BAD_RESPONSE",
            status_code=502,
            detail=decode_response_body(response),
        )

    body = parse_generate_response_json(response)
    content = extract_generate_content(body)

    usage = extract_usage(body)
    resolved_model = str(body.get("model") or model_name)
    finish_reason = str(body.get("done_reason") or "stop")
    return content, resolved_model, usage, latency_ms, finish_reason


def extract_embeddings(body: dict[str, Any], expected_count: int) -> List[List[float]]:
    raw_items: Optional[List[Any]] = None

    if isinstance(body.get("embeddings"), list):
        raw_items = body["embeddings"]
    elif isinstance(body.get("embedding"), list):
        raw_items = [body["embedding"]]

    if raw_items is None:
        raise OllamaClientError(
            "Ollama embed response missing embeddings/embedding field",
            code="OLLAMA_BAD_RESPONSE",
            status_code=502,
            detail=str(list(body.keys())),
        )

    vectors: List[List[float]] = []
    for item in raw_items:
        vector = normalize_vector(item)
        if not vector:
            raise OllamaClientError(
                "Ollama embed returned an empty embedding vector",
                code="OLLAMA_BAD_RESPONSE",
                status_code=502,
            )
        vectors.append(vector)

    if not vectors:
        raise OllamaClientError(
            "Ollama embed returned empty embeddings array",
            code="OLLAMA_BAD_RESPONSE",
            status_code=502,
        )

    if len(vectors) != expected_count:
        raise OllamaClientError(
            f"Ollama embed vector count mismatch: expected {expected_count}, got {len(vectors)}",
            code="OLLAMA_BAD_RESPONSE",
            status_code=502,
            detail=f"embeddings length={len(vectors)}",
        )

    return vectors


def normalize_vector(item: Any) -> List[float]:
    if isinstance(item, list):
        return [float(value) for value in item]
    if isinstance(item, dict):
        for key in ("embedding", "vector", "values"):
            nested = item.get(key)
            if isinstance(nested, list):
                return [float(value) for value in nested]
    raise OllamaClientError(
        "Ollama embed item is not a numeric vector",
        code="OLLAMA_BAD_RESPONSE",
        status_code=502,
        detail=str(type(item)),
    )


def extract_usage(body: dict[str, Any]) -> Optional[dict[str, int]]:
    input_tokens = body.get("prompt_eval_count")
    output_tokens = body.get("eval_count")
    if input_tokens is None and output_tokens is None:
        return None
    return {
        "inputTokens": int(input_tokens or 0),
        "outputTokens": int(output_tokens or 0),
    }


@app.get("/health")
def health() -> dict:
    tags_url = ollama_tags_url()
    reachable, ollama_error = check_ollama_reachable()
    payload = {
        "status": "ok" if reachable else "degraded",
        "provider": PROVIDER,
        "ollamaBaseUrl": OLLAMA_BASE_URL,
        "ollamaTagsUrl": tags_url,
        "ollamaReachable": reachable,
        "ollamaError": ollama_error,
        "embeddingModel": OLLAMA_EMBEDDING_MODEL,
        "llmModel": OLLAMA_LLM_MODEL,
        "timestamp": datetime.now(timezone.utc).isoformat(),
    }
    if not reachable:
        payload["warning"] = (
            "Ollama is not reachable at /api/tags; "
            "worker is up but real model calls will fail until Ollama starts."
        )
    return payload


class EmbedRequest(BaseModel):
    texts: Union[str, List[str]] = Field(default_factory=list)
    model: Optional[str] = None

    @field_validator("texts", mode="before")
    @classmethod
    def coerce_texts(cls, value: Any) -> List[str]:
        if value is None:
            return []
        if isinstance(value, str):
            return [value]
        if isinstance(value, list):
            return value
        raise ValueError("texts must be a string or list of strings")


class EmbedResponse(BaseModel):
    provider: str
    model: str
    dimension: int
    vectors: List[List[float]]
    latencyMs: int


class EmbeddingCompatRequest(BaseModel):
    model: Optional[str] = None
    input: Union[str, List[str]]


class EmbeddingCompatData(BaseModel):
    index: int
    embedding: List[float]


class EmbeddingCompatResponse(BaseModel):
    provider: str
    model: str
    dimension: int
    data: List[EmbeddingCompatData]


class GenerateUsage(BaseModel):
    inputTokens: Optional[int] = None
    outputTokens: Optional[int] = None


class GenerateRequest(BaseModel):
    systemPrompt: Optional[str] = None
    userPrompt: str
    temperature: float = 0.2
    maxTokens: int = 1200
    model: Optional[str] = None


class GenerateResponse(BaseModel):
    provider: str
    model: str
    content: str
    usage: Optional[GenerateUsage] = None
    latencyMs: int
    finishReason: str = "stop"


@app.post("/embed", response_model=EmbedResponse)
def embed(request: EmbedRequest) -> EmbedResponse:
    texts = normalize_texts(request.texts)
    try:
        vectors, model, dimension, latency_ms = embed_texts(texts, request.model)
    except OllamaClientError as exc:
        raise to_http_exception(exc) from exc

    return EmbedResponse(
        provider=PROVIDER,
        model=model,
        dimension=dimension,
        vectors=vectors,
        latencyMs=latency_ms,
    )


@app.post("/embeddings", response_model=EmbeddingCompatResponse)
def embeddings_compat(request: EmbeddingCompatRequest) -> EmbeddingCompatResponse:
    texts = normalize_compat_input(request.input)
    try:
        vectors, model, dimension, _latency_ms = embed_texts(texts, request.model)
    except OllamaClientError as exc:
        raise to_http_exception(exc) from exc

    data = [EmbeddingCompatData(index=index, embedding=vector) for index, vector in enumerate(vectors)]
    return EmbeddingCompatResponse(
        provider=PROVIDER,
        model=model,
        dimension=dimension,
        data=data,
    )


@app.post("/generate")
def generate(request: GenerateRequest) -> UTF8JSONResponse:
    try:
        content, model, usage, latency_ms, finish_reason = generate_text(
            request.systemPrompt,
            request.userPrompt,
            request.model,
            request.temperature,
            request.maxTokens,
        )
    except OllamaClientError as exc:
        raise to_http_exception(exc) from exc

    generate_usage = None
    if usage is not None:
        generate_usage = {
            "inputTokens": usage.get("inputTokens"),
            "outputTokens": usage.get("outputTokens"),
        }

    return UTF8JSONResponse(
        {
            "provider": PROVIDER,
            "model": model,
            "content": content,
            "usage": generate_usage,
            "latencyMs": latency_ms,
            "finishReason": finish_reason,
        }
    )


def to_http_exception(exc: OllamaClientError) -> HTTPException:
    return HTTPException(
        status_code=exc.status_code,
        detail={
            "code": exc.code,
            "message": exc.message,
            "detail": exc.detail,
        },
    )


def normalize_texts(texts: List[str]) -> List[str]:
    if not texts:
        raise HTTPException(
            status_code=400,
            detail={"code": "INVALID_REQUEST", "message": "texts must not be empty", "detail": None},
        )
    normalized: List[str] = []
    for text in texts:
        if text is None or not str(text).strip():
            raise HTTPException(
                status_code=400,
                detail={
                    "code": "INVALID_REQUEST",
                    "message": "text items must be non-blank strings",
                    "detail": None,
                },
            )
        normalized.append(str(text).strip())
    return normalized


def normalize_compat_input(raw_input: Union[str, List[str]]) -> List[str]:
    if isinstance(raw_input, str):
        return normalize_texts([raw_input])
    if isinstance(raw_input, list):
        return normalize_texts(raw_input)
    raise HTTPException(
        status_code=400,
        detail={
            "code": "INVALID_REQUEST",
            "message": "input must be a string or list of strings",
            "detail": None,
        },
    )
