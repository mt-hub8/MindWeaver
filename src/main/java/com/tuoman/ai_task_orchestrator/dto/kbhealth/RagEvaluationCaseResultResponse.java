package com.tuoman.ai_task_orchestrator.dto.kbhealth;

import com.tuoman.ai_task_orchestrator.kbhealth.EvaluationRetrievedChunk;
import com.tuoman.ai_task_orchestrator.kbhealth.HealthMetricValue;
import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationRetrievalStrategy;
import lombok.AllArgsConstructor;
import lombok.Getter;

import java.util.List;
import java.util.Map;

@Getter
@AllArgsConstructor
public class RagEvaluationCaseResultResponse {

    private Long id;

    private Long runId;

    private String caseId;

    private String query;

    private RagEvaluationRetrievalStrategy strategy;

    private Integer topK;

    private List<EvaluationRetrievedChunk> retrievedChunks;

    private String generatedAnswer;

    private List<String> citations;

    private List<HealthMetricValue> retrievalMetrics;

    private List<HealthMetricValue> generationMetrics;

    private Integer qualityScore;

    private Map<String, Object> diagnosis;

    private Long latencyMs;

    private String errorCode;

    private String errorMessage;
}
