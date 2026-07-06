package com.tuoman.ai_task_orchestrator.dto.kbhealth;

import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationRetrievalStrategy;
import com.tuoman.ai_task_orchestrator.kbhealth.RagHealthScoringProfile;
import lombok.Getter;
import lombok.Setter;

import java.util.Map;

@Getter
@Setter
public class CreateRagEvaluationRunRequest {

    private Long datasetId;

    private String name;

    private RagEvaluationRetrievalStrategy strategy;

    private RagHealthScoringProfile scoringProfile;

    private Integer topK;

    private Integer retrievalTopK;

    private Integer rerankTopN;

    private Long collectionId;

    private Map<String, Object> metadataFilter;

    private Boolean executeGeneration;
}
