"""Unit tests for Ollama-backed AI Runtime Worker (no real Ollama required)."""

from unittest.mock import MagicMock, patch

import httpx
import pytest
from fastapi.testclient import TestClient

import main
import ollama_client


@pytest.fixture
def client() -> TestClient:
    return TestClient(main.app)


def test_health_returns_config(client: TestClient) -> None:
    with patch("main.is_ollama_reachable", return_value=True):
        response = client.get("/health")
    assert response.status_code == 200
    body = response.json()
    assert body["provider"] == "local-ollama"
    assert body["embeddingModel"] == ollama_client.OLLAMA_EMBEDDING_MODEL
    assert body["llmModel"] == ollama_client.OLLAMA_LLM_MODEL
    assert body["ollamaBaseUrl"] == ollama_client.OLLAMA_BASE_URL
    assert "timestamp" in body


def test_health_shows_warning_when_ollama_unreachable(client: TestClient) -> None:
    with patch("main.is_ollama_reachable", return_value=False):
        response = client.get("/health")
    assert response.status_code == 200
    body = response.json()
    assert body["status"] == "degraded"
    assert body["ollamaReachable"] is False
    assert "warning" in body


def test_embed_uses_default_embedding_model(client: TestClient) -> None:
    with patch("main.embed_texts", return_value=([[0.1, 0.2]], "qwen3-embedding:0.6b", 2, 10)) as mocked:
        response = client.post("/embed", json={"texts": ["hello"]})
    assert response.status_code == 200
    mocked.assert_called_once()
    assert mocked.call_args.args[1] is None
    body = response.json()
    assert body["provider"] == "local-ollama"
    assert body["model"] == "qwen3-embedding:0.6b"
    assert body["latencyMs"] == 10


def test_generate_uses_default_llm_model(client: TestClient) -> None:
    with patch(
        "main.generate_text",
        return_value=("回答", "qwen2.5:7b", {"inputTokens": 1, "outputTokens": 2}, 20, "stop"),
    ) as mocked:
        response = client.post("/generate", json={"userPrompt": "解释 RAG"})
    assert response.status_code == 200
    mocked.assert_called_once()
    assert mocked.call_args.args[2] is None
    body = response.json()
    assert body["content"] == "回答"
    assert body["model"] == "qwen2.5:7b"


def test_embed_returns_clear_error_when_ollama_unavailable(client: TestClient) -> None:
    with patch(
        "main.embed_texts",
        side_effect=ollama_client.OllamaClientError("Ollama unavailable", code="OLLAMA_UNAVAILABLE", status_code=503),
    ):
        response = client.post("/embed", json={"texts": ["hello"]})
    assert response.status_code == 503
    detail = response.json()["detail"]
    assert detail["code"] == "OLLAMA_UNAVAILABLE"


def test_generate_returns_clear_error_on_bad_response(client: TestClient) -> None:
    with patch(
        "main.generate_text",
        side_effect=ollama_client.OllamaClientError("empty content", code="OLLAMA_BAD_RESPONSE", status_code=502),
    ):
        response = client.post("/generate", json={"userPrompt": "test"})
    assert response.status_code == 502
    detail = response.json()["detail"]
    assert detail["code"] == "OLLAMA_BAD_RESPONSE"


def test_ollama_embed_parses_embeddings_field() -> None:
    mock_response = MagicMock()
    mock_response.status_code = 200
    mock_response.json.return_value = {
        "model": "qwen3-embedding:0.6b",
        "embeddings": [[0.1, 0.2, 0.3]],
    }

    with patch("ollama_client.httpx.Client") as client_cls:
        client = client_cls.return_value.__enter__.return_value
        client.post.return_value = mock_response
        vectors, model, dimension, latency = ollama_client.embed_texts(["text"])

    assert vectors == [[0.1, 0.2, 0.3]]
    assert model == "qwen3-embedding:0.6b"
    assert dimension == 3
    assert latency >= 0


def test_ollama_embed_raises_on_connection_error() -> None:
    with patch("ollama_client.httpx.Client") as client_cls:
        client = client_cls.return_value.__enter__.return_value
        client.post.side_effect = httpx.ConnectError("connection refused")
        with pytest.raises(ollama_client.OllamaClientError) as exc:
            ollama_client.embed_texts(["text"])
    assert exc.value.code == "OLLAMA_UNAVAILABLE"
