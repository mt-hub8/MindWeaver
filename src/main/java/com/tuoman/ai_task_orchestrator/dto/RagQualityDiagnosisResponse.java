package com.tuoman.ai_task_orchestrator.dto;

import com.fasterxml.jackson.annotation.JsonInclude;
import lombok.AllArgsConstructor;
import lombok.Getter;

import java.util.List;

@Getter
@AllArgsConstructor
@JsonInclude(JsonInclude.Include.NON_NULL)
public class RagQualityDiagnosisResponse {

    private String summary;

    private List<RagQualityIssueResponse> issues;

    private List<RagQualitySuggestionResponse> suggestions;
}
