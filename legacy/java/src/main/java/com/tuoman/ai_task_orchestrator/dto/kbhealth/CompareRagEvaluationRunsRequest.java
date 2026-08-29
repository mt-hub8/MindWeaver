package com.tuoman.ai_task_orchestrator.dto.kbhealth;

import lombok.Getter;
import lombok.Setter;

@Getter
@Setter
public class CompareRagEvaluationRunsRequest {

    private Long baselineRunId;

    private Long candidateRunId;
}
