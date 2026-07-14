package com.tuoman.ai_task_orchestrator.embedding;

import java.util.List;

/**
 * Embedding provider 抽象。
 *
 * Java 业务链路只依赖 provider/model/dimension/vector 这组稳定契约，
 * 具体实现可以是 mock、本地 Python/Ollama worker，或 OpenAI-compatible HTTP 服务。
 *
 * LLM provider 与 Embedding provider 分开，是因为回答生成和向量索引的模型、维度、
 * 缓存策略和 reindex 影响都不同。
 */
public interface EmbeddingProvider {

    EmbeddingResponse embed(EmbeddingRequest request);

    default List<EmbeddingResponse> embedBatch(List<EmbeddingRequest> requests) {
        if (requests == null || requests.isEmpty()) {
            return List.of();
        }
        return requests.stream()
                .map(this::embed)
                .toList();
    }

    String provider();

    default String runtimeProvider() {
        return provider();
    }

    String model();

    int dimension();
}
