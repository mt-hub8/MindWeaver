"""Unit tests for Ollama-backed AI Runtime Worker (no real Ollama required)."""

import json
from unittest.mock import MagicMock, patch

import httpx
import pytest
from fastapi.testclient import TestClient

import main


@pytest.fixture
def client() -> TestClient:
    return TestClient(main.app)


def test_health_returns_config(client: TestClient) -> None:
    with patch("main.check_ollama_reachable", return_value=(True, None)):
        response = client.get("/health")
    assert response.status_code == 200
    body = response.json()
    assert body["provider"] == "local-ollama"
    assert body["embeddingModel"] == main.OLLAMA_EMBEDDING_MODEL
    assert body["llmModel"] == main.OLLAMA_LLM_MODEL
    assert body["ollamaBaseUrl"] == main.OLLAMA_BASE_URL
    assert "timestamp" in body


def test_health_shows_warning_when_ollama_unreachable(client: TestClient) -> None:
    with patch("main.check_ollama_reachable", return_value=(False, "HTTPStatusError: HTTP 502")):
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
        side_effect=main.OllamaClientError("Ollama unavailable", code="OLLAMA_UNAVAILABLE", status_code=503),
    ):
        response = client.post("/embed", json={"texts": ["hello"]})
    assert response.status_code == 503
    detail = response.json()["detail"]
    assert detail["code"] == "OLLAMA_UNAVAILABLE"


def test_generate_returns_clear_error_on_bad_response(client: TestClient) -> None:
    with patch(
        "main.generate_text",
        side_effect=main.OllamaClientError("empty content", code="OLLAMA_BAD_RESPONSE", status_code=502),
    ):
        response = client.post("/generate", json={"userPrompt": "test"})
    assert response.status_code == 502
    detail = response.json()["detail"]
    assert detail["code"] == "OLLAMA_BAD_RESPONSE"


def test_embed_calls_ollama_once_per_text_with_string_input() -> None:
    mock_response = MagicMock()
    mock_response.status_code = 200
    mock_response.json.return_value = {
        "model": "qwen3-embedding:0.6b",
        "embeddings": [[0.1, 0.2, 0.3]],
    }

    with patch("main.httpx.Client") as client_cls:
        client = client_cls.return_value.__enter__.return_value
        client.post.return_value = mock_response
        vectors, model, dimension, latency = main.embed_texts(["text one", "text two"])

    assert client.post.call_count == 2
    first_payload = client.post.call_args_list[0].kwargs["json"]
    second_payload = client.post.call_args_list[1].kwargs["json"]
    assert first_payload == {"model": "qwen3-embedding:0.6b", "input": "text one"}
    assert second_payload == {"model": "qwen3-embedding:0.6b", "input": "text two"}
    assert vectors == [[0.1, 0.2, 0.3], [0.1, 0.2, 0.3]]
    assert model == "qwen3-embedding:0.6b"
    assert dimension == 3
    assert latency >= 0


def test_embed_parses_embeddings_zero_index() -> None:
    mock_response = MagicMock()
    mock_response.status_code = 200
    mock_response.json.return_value = {
        "model": "qwen3-embedding:0.6b",
        "embeddings": [[0.1, 0.2, 0.3]],
    }

    with patch("main.httpx.Client") as client_cls:
        client = client_cls.return_value.__enter__.return_value
        client.post.return_value = mock_response
        vectors, model, dimension, latency = main.embed_texts(["text"])

    assert vectors == [[0.1, 0.2, 0.3]]
    assert model == "qwen3-embedding:0.6b"
    assert dimension == 3
    assert latency >= 0


def test_embed_bad_response_includes_ollama_detail() -> None:
    mock_response = MagicMock()
    mock_response.status_code = 502
    mock_response.text = "Bad Gateway from proxy"

    with patch("main.httpx.Client") as client_cls:
        client = client_cls.return_value.__enter__.return_value
        client.post.return_value = mock_response
        with pytest.raises(main.OllamaClientError) as exc:
            main.embed_texts(["test embedding"])

    detail = exc.value.detail
    assert detail["ollamaUrl"] == main.ollama_embed_url()
    assert detail["model"] == "qwen3-embedding:0.6b"
    assert detail["inputLength"] == len("test embedding")
    assert detail["statusCode"] == 502
    assert detail["responseBody"] == "Bad Gateway from proxy"


def test_embeddings_compat_reuses_embed_texts(client: TestClient) -> None:
    with patch(
        "main.embed_texts",
        return_value=([[0.4, 0.5]], "qwen3-embedding:0.6b", 2, 7),
    ) as mocked:
        response = client.post("/embeddings", json={"input": ["compat text"]})
    assert response.status_code == 200
    mocked.assert_called_once_with(["compat text"], None)
    body = response.json()
    assert body["provider"] == "local-ollama"
    assert body["model"] == "qwen3-embedding:0.6b"
    assert body["dimension"] == 2
    assert body["data"][0]["embedding"] == [0.4, 0.5]


def test_ollama_embed_raises_on_connection_error() -> None:
    with patch("main.httpx.Client") as client_cls:
        client = client_cls.return_value.__enter__.return_value
        client.post.side_effect = httpx.ConnectError("connection refused")
        with pytest.raises(main.OllamaClientError) as exc:
            main.embed_texts(["text"])
    assert exc.value.code == "OLLAMA_UNAVAILABLE"


def test_generate_preserves_chinese_from_utf8_response_bytes() -> None:
    chinese = "我是 Qwen，一个由阿里云开发的大语言模型。"
    body_bytes = json.dumps(
        {
            "model": "qwen2.5:7b",
            "response": chinese,
            "done_reason": "stop",
        },
        ensure_ascii=False,
    ).encode("utf-8")

    mock_response = MagicMock()
    mock_response.status_code = 200
    mock_response.content = body_bytes
    # Broken latin-1 view of the same bytes must not be used for JSON parsing.
    mock_response.text = body_bytes.decode("latin-1")

    with patch("main.httpx.Client") as client_cls:
        client = client_cls.return_value.__enter__.return_value
        client.post.return_value = mock_response
        content, model, _usage, _latency, finish_reason = main.generate_text(
            "你是一个中文技术助手，请根据用户问题直接回答。",
            "请用一句话介绍你自己。",
            "qwen2.5:7b",
        )

    assert content == chinese
    assert model == "qwen2.5:7b"
    assert finish_reason == "stop"


def test_parse_generate_response_json_ignores_response_text_encoding() -> None:
    chinese = "我是 Qwen，一个由阿里云开发的大语言模型。"
    body_bytes = json.dumps(
        {"model": "qwen2.5:7b", "response": chinese, "done_reason": "stop"},
        ensure_ascii=False,
    ).encode("utf-8")

    mock_response = MagicMock()
    mock_response.content = body_bytes
    # If response.text (latin-1) were used for JSON parsing, Chinese would become mojibake.
    mock_response.text = body_bytes.decode("latin-1")

    parsed = main.parse_generate_response_json(mock_response)
    assert main.extract_generate_content(parsed) == chinese


def test_generate_endpoint_returns_utf8_chinese_content(client: TestClient) -> None:
    chinese = "我是 Qwen，一个由阿里云开发的大语言模型。"
    with patch(
        "main.generate_text",
        return_value=(chinese, "qwen2.5:7b", None, 15, "stop"),
    ):
        response = client.post(
            "/generate",
            json={
                "systemPrompt": "你是一个中文技术助手，请根据用户问题直接回答。",
                "userPrompt": "请用一句话介绍你自己。",
                "temperature": 0.2,
                "maxTokens": 300,
                "model": "qwen2.5:7b",
            },
        )

    assert response.status_code == 200
    assert "charset=utf-8" in response.headers["content-type"].lower()
    body = response.json()
    assert body["content"] == chinese
    assert "我是" in response.content.decode("utf-8")
