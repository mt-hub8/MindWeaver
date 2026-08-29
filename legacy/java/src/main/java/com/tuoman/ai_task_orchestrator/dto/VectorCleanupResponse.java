package com.tuoman.ai_task_orchestrator.dto;

import lombok.Builder;
import lombok.Getter;

import java.util.List;

@Getter
@Builder
public class VectorCleanupResponse {
    private final int requestedCount;
    private final int deletedCount;
    private final int failedCount;
    private final int skippedCount;
    private final List<String> warnings;
    private final List<String> errors;
}
