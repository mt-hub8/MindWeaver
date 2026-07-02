package com.tuoman.ai_task_orchestrator.llm;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import lombok.Getter;
import lombok.Setter;

@Getter
@Setter
@JsonIgnoreProperties(ignoreUnknown = true)
public class LocalPythonLlmResponse {

    private String provider;

    private String model;

    private String content;

    private Usage usage;

    private Long latencyMs;

    private String finishReason;

    @Getter
    @Setter
    @JsonIgnoreProperties(ignoreUnknown = true)
    public static class Usage {

        private Integer inputTokens;

        private Integer outputTokens;
    }
}
