package com.tuoman.ai_task_orchestrator.llm;

import lombok.Getter;
import lombok.Setter;

@Getter
@Setter
public class LlmGenerateOptions {

    private String model;

    private Double temperature;

    private Integer maxTokens;

    private Long taskId;
}
