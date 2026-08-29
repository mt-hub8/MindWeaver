package com.tuoman.ai_task_orchestrator.embedding;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Component;

import java.util.List;

@Component
@RequiredArgsConstructor
public class EmbeddingCacheJsonCodec {

    private static final TypeReference<List<Double>> VECTOR_TYPE = new TypeReference<>() {
    };

    private final ObjectMapper objectMapper;

    public String serialize(List<Double> vector) {
        if (vector == null || vector.isEmpty()) {
            throw new EmbeddingProviderException("embedding vector must not be empty");
        }
        try {
            return objectMapper.writeValueAsString(vector);
        } catch (JsonProcessingException exception) {
            throw new EmbeddingProviderException("failed to serialize embedding vector", exception);
        }
    }

    public List<Double> deserialize(String embeddingJson) {
        if (embeddingJson == null || embeddingJson.isBlank()) {
            throw new EmbeddingProviderException("embedding_json must not be blank");
        }
        try {
            List<Double> vector = objectMapper.readValue(embeddingJson, VECTOR_TYPE);
            if (vector == null || vector.isEmpty()) {
                throw new EmbeddingProviderException("embedding_json deserialized to empty vector");
            }
            return vector;
        } catch (JsonProcessingException exception) {
            throw new EmbeddingProviderException("failed to deserialize embedding_json", exception);
        }
    }
}
