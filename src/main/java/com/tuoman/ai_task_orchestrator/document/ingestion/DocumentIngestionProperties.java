package com.tuoman.ai_task_orchestrator.document.ingestion;

import lombok.Getter;
import lombok.Setter;
import org.springframework.boot.context.properties.ConfigurationProperties;
import org.springframework.stereotype.Component;

@Getter
@Setter
@Component
@ConfigurationProperties(prefix = "app.document.ingestion")
public class DocumentIngestionProperties {

    private long maxFileSizeBytes = 2 * 1024 * 1024;
}
