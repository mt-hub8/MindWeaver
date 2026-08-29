package com.tuoman.ai_task_orchestrator.dto.kbhealth;

import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationRetrievalStrategy;
import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationRunStatus;
import com.tuoman.ai_task_orchestrator.kbhealth.RagHealthScoringProfile;
import lombok.AllArgsConstructor;
import lombok.Getter;

import java.time.LocalDateTime;
import java.util.Map;

@Getter
@AllArgsConstructor
public class RagEvaluationRunResponse {

    private Long runId;

    private Long datasetId;

    private String name;

    private RagEvaluationRunStatus status;

    private RagEvaluationRetrievalStrategy strategy;

    private String strategyDisplayName;

    private RagHealthScoringProfile scoringProfile;

    private String scoringProfileDisplayName;

    private Integer topK;

    private Integer retrievalTopK;

    private Integer rerankTopN;

    private Long collectionId;

    private Map<String, Object> metadataFilter;

    private Boolean executeGeneration;

    private LocalDateTime startedAt;

    private LocalDateTime completedAt;

    private Integer totalCases;

    private Integer completedCases;

    private Integer failedCases;

    private Integer overallScore;

    private String overallScoreLevel;

    private Map<String, Object> summary;

    private Map<String, Object> diagnosis;
}
