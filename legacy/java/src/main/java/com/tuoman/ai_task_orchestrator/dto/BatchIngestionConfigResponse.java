package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class BatchIngestionConfigResponse {

    private boolean enabled;

    private int maxFilesPerBatch;

    private int documentParseConcurrency;

    private int embeddingConcurrency;

    private int maxRetryCount;

    private String stagingDir;
}
