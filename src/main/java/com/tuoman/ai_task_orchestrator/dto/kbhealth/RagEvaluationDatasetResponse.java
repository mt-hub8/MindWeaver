package com.tuoman.ai_task_orchestrator.dto.kbhealth;

import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationDatasetStatus;
import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationDatasetType;
import lombok.AllArgsConstructor;
import lombok.Getter;

import java.time.LocalDateTime;

@Getter
@AllArgsConstructor
public class RagEvaluationDatasetResponse {

    private Long datasetId;

    private String name;

    private String description;

    private RagEvaluationDatasetType datasetType;

    private String datasetTypeDisplayName;

    private RagEvaluationDatasetStatus status;

    private Integer caseCount;

    private String sourceType;

    private LocalDateTime createdAt;

    private LocalDateTime updatedAt;
}
