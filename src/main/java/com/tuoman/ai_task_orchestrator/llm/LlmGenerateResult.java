package com.tuoman.ai_task_orchestrator.llm;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.util.Map;

@Getter
@AllArgsConstructor
public class LlmGenerateResult {

    private final String content;

    private final String provider;

    private final String model;

    private final Integer inputTokens;

    private final Integer outputTokens;

    private final Long latencyMs;

    private final String finishReason;

    private final Map<String, Object> rawMetadata;

    private final boolean success;

    private final String errorMessage;

    public static LlmGenerateResult success(
            String content,
            String provider,
            String model,
            Integer inputTokens,
            Integer outputTokens,
            Long latencyMs,
            String finishReason,
            Map<String, Object> rawMetadata
    ) {
        return new LlmGenerateResult(
                content,
                provider,
                model,
                inputTokens,
                outputTokens,
                latencyMs,
                finishReason,
                rawMetadata,
                true,
                null
        );
    }

    public static LlmGenerateResult failure(String provider, String model, String errorMessage, Long latencyMs) {
        return new LlmGenerateResult(
                null,
                provider,
                model,
                null,
                null,
                latencyMs,
                null,
                null,
                false,
                errorMessage
        );
    }
}
