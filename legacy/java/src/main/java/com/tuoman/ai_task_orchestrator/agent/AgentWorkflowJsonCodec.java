package com.tuoman.ai_task_orchestrator.agent;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Component;

import java.util.LinkedHashMap;
import java.util.Map;

@Component
@RequiredArgsConstructor
public class AgentWorkflowJsonCodec {

    private static final int JSON_MAX = 16000;

    private final ObjectMapper objectMapper;

    public String serialize(Map<String, Object> value) {
        if (value == null || value.isEmpty()) {
            return null;
        }
        try {
            String json = objectMapper.writeValueAsString(value);
            if (json.length() <= JSON_MAX) {
                return json;
            }
            return json.substring(0, JSON_MAX) + "...[truncated]";
        } catch (JsonProcessingException exception) {
            return "{\"serializationError\":true}";
        }
    }

    public Map<String, Object> deserialize(String json) {
        if (json == null || json.isBlank()) {
            return Map.of();
        }
        try {
            return objectMapper.readValue(json, new TypeReference<LinkedHashMap<String, Object>>() {
            });
        } catch (JsonProcessingException exception) {
            return Map.of("raw", json);
        }
    }
}
