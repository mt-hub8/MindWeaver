package com.tuoman.ai_task_orchestrator.dto.kbhealth;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.util.List;
import java.util.Map;

@Getter
@AllArgsConstructor
public class CompareRagEvaluationRunsResponse {

    private Integer baselineScore;

    private Integer candidateScore;

    private Integer deltaScore;

    private Map<String, Double> metricDeltas;

    private List<String> improvedMetrics;

    private List<String> regressedMetrics;

    private String summary;

    private List<String> suggestions;
}
