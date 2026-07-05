package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class TrashCleanupResponse {

    private int successCount;

    private int failureCount;

    private String message;
}
