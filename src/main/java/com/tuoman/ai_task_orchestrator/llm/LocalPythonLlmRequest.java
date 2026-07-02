package com.tuoman.ai_task_orchestrator.llm;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import lombok.Getter;
import lombok.Setter;

@Getter
@Setter
@JsonIgnoreProperties(ignoreUnknown = true)
public class LocalPythonLlmRequest {

    private String systemPrompt;

    private String userPrompt;

    private Double temperature;

    private Integer maxTokens;

    private String model;
}
