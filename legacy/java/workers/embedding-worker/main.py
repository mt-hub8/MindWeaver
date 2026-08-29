from functools import lru_cache
from typing import List, Union

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from sentence_transformers import SentenceTransformer


PROVIDER = "local-worker"

app = FastAPI(title="AI Task Orchestrator Embedding Worker")


@app.get("/health")
def health() -> dict[str, str]:
    return {"status": "ok", "provider": PROVIDER}


class EmbeddingRequest(BaseModel):
    model: str
    input: Union[str, List[str]]


class EmbeddingData(BaseModel):
    index: int
    embedding: List[float]


class EmbeddingResponse(BaseModel):
    provider: str
    model: str
    dimension: int
    data: List[EmbeddingData]


@lru_cache(maxsize=4)
def load_model(model_name: str) -> SentenceTransformer:
    try:
        return SentenceTransformer(model_name)
    except Exception as exc:
        raise HTTPException(status_code=500, detail=f"failed to load embedding model: {exc}") from exc


@app.post("/embeddings", response_model=EmbeddingResponse)
def create_embeddings(request: EmbeddingRequest) -> EmbeddingResponse:
    if request.model is None or not request.model.strip():
        raise HTTPException(status_code=400, detail="model must not be blank")

    texts = normalize_input(request.input)
    model = load_model(request.model)

    try:
        vectors = model.encode(texts, convert_to_numpy=False)
    except Exception as exc:
        raise HTTPException(status_code=500, detail=f"failed to create embeddings: {exc}") from exc

    if len(vectors) != len(texts):
        raise HTTPException(status_code=500, detail="embedding result size does not match input size")

    data = []
    dimension = None
    for index, vector in enumerate(vectors):
        embedding = [float(value) for value in vector]
        if not embedding:
            raise HTTPException(status_code=500, detail="embedding vector must not be empty")
        if dimension is None:
            dimension = len(embedding)
        data.append(EmbeddingData(index=index, embedding=embedding))

    return EmbeddingResponse(
        provider=PROVIDER,
        model=request.model,
        dimension=dimension or 0,
        data=data,
    )


def normalize_input(raw_input: Union[str, List[str]]) -> List[str]:
    if isinstance(raw_input, str):
        if not raw_input.strip():
            raise HTTPException(status_code=400, detail="input must not be blank")
        return [raw_input]

    if isinstance(raw_input, list):
        if not raw_input:
            raise HTTPException(status_code=400, detail="input must not be empty")
        if any(item is None or not isinstance(item, str) or not item.strip() for item in raw_input):
            raise HTTPException(status_code=400, detail="input items must be non-blank strings")
        return raw_input

    raise HTTPException(status_code=400, detail="input must be a string or list of strings")
