package com.tuoman.ai_task_orchestrator.config;

import lombok.Getter;
import lombok.Setter;
import org.springframework.boot.context.properties.ConfigurationProperties;
import org.springframework.stereotype.Component;

@Getter
@Setter
@Component
@ConfigurationProperties(prefix = "app.batch-ingestion")
public class BatchIngestionProperties {

    private boolean enabled = true;

    private int maxFilesPerBatch = 100;

    private int documentParseConcurrency = 2;

    private int embeddingConcurrency = 1;

    private int maxRetryCount = 2;

    private String stagingDir = "data/staging/batches";
}
