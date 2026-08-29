package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class AgentTaskModelMetadataResponse {

    private String llmProvider;

    private String llmModel;

    private String embeddingProvider;

    private String embeddingModel;

    private Integer inputTokens;

    private Integer outputTokens;

    private Integer retrievalCount;

    private Integer citationCount;
}
