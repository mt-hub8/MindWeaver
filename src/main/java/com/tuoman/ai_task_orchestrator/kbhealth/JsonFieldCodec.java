package com.tuoman.ai_task_orchestrator.kbhealth;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;

import java.util.Collections;
import java.util.List;
import java.util.Map;

public final class JsonFieldCodec {

    private static final ObjectMapper MAPPER = new ObjectMapper();

    private JsonFieldCodec() {
    }

    public static String write(Object value) {
        if (value == null) {
            return null;
        }
        try {
            return MAPPER.writeValueAsString(value);
        } catch (Exception exception) {
            throw new IllegalStateException("Failed to serialize JSON field", exception);
        }
    }

    public static List<String> readStringList(String json) {
        if (json == null || json.isBlank()) {
            return List.of();
        }
        try {
            return MAPPER.readValue(json, new TypeReference<List<String>>() {
            });
        } catch (Exception exception) {
            return List.of();
        }
    }

    public static List<Long> readLongList(String json) {
        List<String> raw = readStringList(json);
        if (raw.isEmpty()) {
            try {
                return MAPPER.readValue(json, new TypeReference<List<Long>>() {
                });
            } catch (Exception ignored) {
                return List.of();
            }
        }
        return raw.stream()
                .map(JsonFieldCodec::parseLongSafe)
                .filter(id -> id != null)
                .toList();
    }

    public static Map<String, Object> readMap(String json) {
        if (json == null || json.isBlank()) {
            return Collections.emptyMap();
        }
        try {
            return MAPPER.readValue(json, new TypeReference<Map<String, Object>>() {
            });
        } catch (Exception exception) {
            return Collections.emptyMap();
        }
    }

    public static <T> T readObject(String json, Class<T> type) {
        if (json == null || json.isBlank()) {
            return null;
        }
        try {
            return MAPPER.readValue(json, type);
        } catch (Exception exception) {
            return null;
        }
    }

    public static <T> T readObject(String json, TypeReference<T> type) {
        if (json == null || json.isBlank()) {
            return null;
        }
        try {
            return MAPPER.readValue(json, type);
        } catch (Exception exception) {
            return null;
        }
    }

    private static Long parseLongSafe(String value) {
        if (value == null || value.isBlank()) {
            return null;
        }
        try {
            return Long.parseLong(value);
        } catch (NumberFormatException exception) {
            return null;
        }
    }
}
