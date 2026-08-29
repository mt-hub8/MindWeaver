"""Ollama HTTP client for AI Runtime Worker."""

from __future__ import annotations

import os
import time
from typing import Any, List, Optional, Tuple

import httpx

PROVIDER = "local-ollama"

OLLAMA_BASE_URL = os.getenv("OLLAMA_BASE_URL", "http://localhost:11434").rstrip("/")
OLLAMA_EMBEDDING_MODEL = os.getenv("OLLAMA_EMBEDDING_MODEL", "qwen3-embedding:0.6b")
OLLAMA_LLM_MODEL = os.getenv("OLLAMA_LLM_MODEL", "qwen2.5:7b")
OLLAMA_TIMEOUT_SECONDS = float(os.getenv("OLLAMA_TIMEOUT_SECONDS", "120"))


class OllamaClientError(Exception):
    def __init__(self, message: str, code: str = "OLLAMA_ERROR", status_code: int = 502) -> None:
        super().__init__(message)
        self.code = code
        self.status_code = status_code


def is_ollama_reachable() -> bool:
    try:
        with httpx.Client(timeout=5.0) as client:
            response = client.get(f"{OLLAMA_BASE_URL}/api/tags")
            return response.status_code == 200
    except Exception:
        return False


def embed_texts(texts: List[str], model: Optional[str] = None) -> Tuple[List[List[float]], str, int, int]:
    if not texts:
        raise OllamaClientError("texts must not be empty", code="INVALID_REQUEST", status_code=400)

    model_name = (model or OLLAMA_EMBEDDING_MODEL).strip()
    if not model_name:
        raise OllamaClientError("model must not be blank", code="INVALID_REQUEST", status_code=400)

    payload: dict[str, Any] = {
        "model": model_name,
        "input": texts[0] if len(texts) == 1 else texts,
    }

    started = time.perf_counter()
    try:
        with httpx.Client(timeout=OLLAMA_TIMEOUT_SECONDS) as client:
            response = client.post(f"{OLLAMA_BASE_URL}/api/embed", json=payload)
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
        ) from exc

    latency_ms = int((time.perf_counter() - started) * 1000)
    if response.status_code >= 400:
        raise OllamaClientError(
            f"Ollama embed failed ({response.status_code}): {response.text}",
            code="OLLAMA_BAD_RESPONSE",
            status_code=502,
        )

    try:
        body = response.json()
    except ValueError as exc:
        raise OllamaClientError(
            "Ollama embed returned non-JSON response",
            code="OLLAMA_BAD_RESPONSE",
            status_code=502,
        ) from exc

    vectors = extract_embeddings(body, len(texts))
    dimension = len(vectors[0]) if vectors else 0
    resolved_model = str(body.get("model") or model_name)
    return vectors, resolved_model, dimension, latency_ms


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
        with httpx.Client(timeout=OLLAMA_TIMEOUT_SECONDS) as client:
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
        ) from exc

    latency_ms = int((time.perf_counter() - started) * 1000)
    if response.status_code >= 400:
        raise OllamaClientError(
            f"Ollama generate failed ({response.status_code}): {response.text}",
            code="OLLAMA_BAD_RESPONSE",
            status_code=502,
        )

    try:
        body = response.json()
    except ValueError as exc:
        raise OllamaClientError(
            "Ollama generate returned non-JSON response",
            code="OLLAMA_BAD_RESPONSE",
            status_code=502,
        ) from exc

    content = body.get("response")
    if content is None or not str(content).strip():
        raise OllamaClientError(
            "Ollama generate returned empty content",
            code="OLLAMA_BAD_RESPONSE",
            status_code=502,
        )

    usage = extract_usage(body)
    resolved_model = str(body.get("model") or model_name)
    finish_reason = str(body.get("done_reason") or "stop")
    return str(content), resolved_model, usage, latency_ms, finish_reason


def extract_embeddings(body: dict[str, Any], expected_count: int) -> List[List[float]]:
    if "embeddings" in body and isinstance(body["embeddings"], list):
        vectors = [[float(v) for v in item] for item in body["embeddings"]]
    elif "embedding" in body and isinstance(body["embedding"], list):
        vectors = [[float(v) for v in body["embedding"]]]
    else:
        raise OllamaClientError(
            "Ollama embed response missing embeddings/embedding field",
            code="OLLAMA_BAD_RESPONSE",
            status_code=502,
        )

    if not vectors or any(not vector for vector in vectors):
        raise OllamaClientError(
            "Ollama embed returned empty vectors",
            code="OLLAMA_BAD_RESPONSE",
            status_code=502,
        )
    if len(vectors) != expected_count:
        raise OllamaClientError(
            f"Ollama embed vector count mismatch: expected {expected_count}, got {len(vectors)}",
            code="OLLAMA_BAD_RESPONSE",
            status_code=502,
        )
    return vectors


def extract_usage(body: dict[str, Any]) -> Optional[dict[str, int]]:
    input_tokens = body.get("prompt_eval_count")
    output_tokens = body.get("eval_count")
    if input_tokens is None and output_tokens is None:
        return None
    return {
        "inputTokens": int(input_tokens or 0),
        "outputTokens": int(output_tokens or 0),
    }
