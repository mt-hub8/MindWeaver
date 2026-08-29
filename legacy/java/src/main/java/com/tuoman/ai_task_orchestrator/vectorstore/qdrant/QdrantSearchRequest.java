package com.tuoman.ai_task_orchestrator.vectorstore.qdrant;

import com.fasterxml.jackson.annotation.JsonProperty;

import java.util.List;

public record QdrantSearchRequest(
        List<Double> vector,
        Integer limit,
        @JsonProperty("with_payload")
        Boolean withPayload,
        QdrantFilter filter
) {
}
