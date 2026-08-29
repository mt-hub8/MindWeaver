package com.tuoman.ai_task_orchestrator.dto;

import com.fasterxml.jackson.annotation.JsonInclude;
import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
@JsonInclude(JsonInclude.Include.NON_NULL)
public class RuntimeStatusResponse {

    private String activeProfile;

    private String embeddingProvider;

    private String embeddingModel;

    private Integer embeddingDimension;

    private String llmProvider;

    private String llmModel;

    private String pythonWorkerBaseUrl;

    private Boolean pythonWorkerReachable;

    private String ollamaBaseUrl;

    private Boolean ollamaReachable;

    private String vectorStoreProvider;

    private String statusMessage;
}
