package com.tuoman.ai_task_orchestrator.llm;

import lombok.Getter;
import lombok.Setter;

@Getter
@Setter
public class LlmResponse {

    private Long taskId;

    private String model;

    private String provider;

    private String content;

    private boolean success;

    private String errorMessage;

    private Integer promptTokenCount;

    private Integer completionTokenCount;

    private Integer totalTokenCount;

    private Long latencyMs;
}
