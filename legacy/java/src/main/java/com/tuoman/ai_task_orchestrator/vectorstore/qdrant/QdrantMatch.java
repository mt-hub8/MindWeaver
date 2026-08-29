package com.tuoman.ai_task_orchestrator.vectorstore.qdrant;

import com.fasterxml.jackson.annotation.JsonInclude;

import java.util.List;

@JsonInclude(JsonInclude.Include.NON_NULL)
public record QdrantMatch(
        Object value,
        List<?> any
) {

    public static QdrantMatch value(Object value) {
        return new QdrantMatch(value, null);
    }

    public static QdrantMatch any(List<?> values) {
        return new QdrantMatch(null, values);
    }
}
