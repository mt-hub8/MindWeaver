package com.tuoman.ai_task_orchestrator.dto.kbhealth;

import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationDatasetStatus;
import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationDatasetType;
import lombok.Getter;
import lombok.Setter;

@Getter
@Setter
public class CreateRagEvaluationDatasetRequest {

    private String name;

    private String description;

    private RagEvaluationDatasetType datasetType;

    private RagEvaluationDatasetStatus status;

    private String sourceType;
}
