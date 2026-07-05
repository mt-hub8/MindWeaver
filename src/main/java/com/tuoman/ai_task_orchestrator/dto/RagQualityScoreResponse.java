package com.tuoman.ai_task_orchestrator.dto;

import com.fasterxml.jackson.annotation.JsonInclude;
import com.tuoman.ai_task_orchestrator.rag.quality.RagQualityLevel;
import com.tuoman.ai_task_orchestrator.rag.quality.RagQualityMode;
import lombok.AllArgsConstructor;
import lombok.Getter;

import java.util.Map;

@Getter
@AllArgsConstructor
@JsonInclude(JsonInclude.Include.NON_NULL)
public class RagQualityScoreResponse {

    private Integer overallScore;

    private RagQualityLevel overallLevel;

    private String displayLevel;

    private Integer retrievalScore;

    private Integer contextScore;

    private Integer answerScore;

    private Integer citationScore;

    private RagQualityMode mode;

    private String displayMode;

    private Map<String, Double> weights;

    private RagQualityDiagnosisResponse diagnosis;

    private Map<String, Object> metricDetails;

    private String scoringNote;
}
