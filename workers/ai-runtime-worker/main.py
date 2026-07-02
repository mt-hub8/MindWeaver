from functools import lru_cache
from typing import List, Optional, Union

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field
from sentence_transformers import SentenceTransformer


PROVIDER = "local-python"
DEFAULT_EMBEDDING_MODEL = "sentence-transformers/all-MiniLM-L6-v2"
DEFAULT_LLM_MODEL = "mock-llm-template"

app = FastAPI(title="AI Task Orchestrator AI Runtime Worker")


@app.get("/health")
def health() -> dict:
    return {
        "status": "ok",
        "provider": PROVIDER,
        "availableModels": {
            "embedding": [DEFAULT_EMBEDDING_MODEL],
            "llm": [DEFAULT_LLM_MODEL],
        },
    }


class EmbedRequest(BaseModel):
    texts: List[str] = Field(default_factory=list)
    model: Optional[str] = None


class EmbedResponse(BaseModel):
    provider: str
    model: str
    dimension: int
    vectors: List[List[float]]


class EmbeddingCompatRequest(BaseModel):
    model: str
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
    inputTokens: int = 0
    outputTokens: int = 0


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
    usage: GenerateUsage
    latencyMs: int
    finishReason: str = "stop"


@lru_cache(maxsize=4)
def load_embedding_model(model_name: str) -> SentenceTransformer:
    try:
        return SentenceTransformer(model_name)
    except Exception as exc:
        raise HTTPException(status_code=500, detail=f"failed to load embedding model: {exc}") from exc


@app.post("/embed", response_model=EmbedResponse)
def embed(request: EmbedRequest) -> EmbedResponse:
    model_name = (request.model or DEFAULT_EMBEDDING_MODEL).strip()
    if not model_name:
        raise HTTPException(status_code=400, detail="model must not be blank")
    texts = normalize_texts(request.texts)
    model = load_embedding_model(model_name)
    try:
        vectors = model.encode(texts, convert_to_numpy=False)
    except Exception as exc:
        raise HTTPException(status_code=500, detail=f"failed to create embeddings: {exc}") from exc

    normalized = []
    dimension = 0
    for vector in vectors:
        embedding = [float(value) for value in vector]
        if not embedding:
            raise HTTPException(status_code=500, detail="embedding vector must not be empty")
        dimension = len(embedding)
        normalized.append(embedding)

    return EmbedResponse(
        provider=PROVIDER,
        model=model_name,
        dimension=dimension,
        vectors=normalized,
    )


@app.post("/embeddings", response_model=EmbeddingCompatResponse)
def embeddings_compat(request: EmbeddingCompatRequest) -> EmbeddingCompatResponse:
    texts = normalize_compat_input(request.input)
    embed_response = embed(EmbedRequest(texts=texts, model=request.model))
    data = [
        EmbeddingCompatData(index=index, embedding=vector)
        for index, vector in enumerate(embed_response.vectors)
    ]
    return EmbeddingCompatResponse(
        provider=embed_response.provider,
        model=embed_response.model,
        dimension=embed_response.dimension,
        data=data,
    )


@app.post("/generate", response_model=GenerateResponse)
def generate(request: GenerateRequest) -> GenerateResponse:
    import time

    started = time.perf_counter()
    user_prompt = request.userPrompt.strip() if request.userPrompt else ""
    if not user_prompt:
        raise HTTPException(status_code=400, detail="userPrompt must not be blank")

    model_name = (request.model or DEFAULT_LLM_MODEL).strip()
    system_prompt = request.systemPrompt.strip() if request.systemPrompt else ""

    if "fail" in user_prompt.lower() or "失败" in user_prompt:
        raise HTTPException(status_code=500, detail="mock llm generation failed")

    content = build_mock_llm_content(system_prompt, user_prompt)
    input_tokens = estimate_tokens(system_prompt + user_prompt)
    output_tokens = estimate_tokens(content)
    latency_ms = int((time.perf_counter() - started) * 1000)

    return GenerateResponse(
        provider=PROVIDER,
        model=model_name,
        content=content,
        usage=GenerateUsage(inputTokens=input_tokens, outputTokens=output_tokens),
        latencyMs=latency_ms,
        finishReason="stop",
    )


def build_mock_llm_content(system_prompt: str, user_prompt: str) -> str:
    lines = [
        "## 任务结论",
        "基于提供的知识库上下文，已完成任务分析（本地 CPU 模板生成，非真实大模型）。",
        "",
        "## 关键依据",
        "- 已参考检索到的文档片段与引用编号。",
        "",
        "## 风险 / 不确定性",
        "- 若上下文不足，结论可能不完整，请结合原始文档复核。",
        "",
        "## 下一步建议",
        "- 补充更多相关文档到知识库分组后重新执行任务。",
        "",
        "## 引用来源",
        "- 请查看任务详情中的引用来源列表。",
    ]
    if system_prompt:
        lines.insert(1, "（已应用系统指令）")
    if "任务目标" in user_prompt or "objective" in user_prompt.lower():
        lines.insert(2, "任务目标已在提示词中提供。")
    return "\n".join(lines)


def normalize_texts(texts: List[str]) -> List[str]:
    if not texts:
        raise HTTPException(status_code=400, detail="texts must not be empty")
    normalized = []
    for text in texts:
        if text is None or not str(text).strip():
            raise HTTPException(status_code=400, detail="text items must be non-blank strings")
        normalized.append(str(text).strip())
    return normalized


def normalize_compat_input(raw_input: Union[str, List[str]]) -> List[str]:
    if isinstance(raw_input, str):
        return normalize_texts([raw_input])
    if isinstance(raw_input, list):
        return normalize_texts(raw_input)
    raise HTTPException(status_code=400, detail="input must be a string or list of strings")


def estimate_tokens(text: str) -> int:
    if not text:
        return 0
    return max(1, len(text) // 4)
