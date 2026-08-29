package com.tuoman.ai_task_orchestrator.dto.kbhealth;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.util.List;

@Getter
@AllArgsConstructor
public class ImportRagEvaluationCasesResponse {

    private int totalCount;

    private int importedCount;

    private int skippedCount;

    private int failedCount;

    private List<String> errors;
}
