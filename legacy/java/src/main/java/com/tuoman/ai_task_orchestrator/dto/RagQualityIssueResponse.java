package com.tuoman.ai_task_orchestrator.dto;

import com.fasterxml.jackson.annotation.JsonInclude;
import lombok.AllArgsConstructor;
import lombok.Getter;

import java.util.List;
import java.util.Map;

@Getter
@AllArgsConstructor
@JsonInclude(JsonInclude.Include.NON_NULL)
public class RagQualityIssueResponse {

    private String code;

    private String title;

    private String description;

    private String severity;

    private String relatedMetric;

    private String scoreImpact;
}
