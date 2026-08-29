package com.tuoman.ai_task_orchestrator.queryunderstanding;

import lombok.Getter;
import lombok.Setter;
import org.springframework.boot.context.properties.ConfigurationProperties;
import org.springframework.stereotype.Component;

@Getter
@Setter
@Component
@ConfigurationProperties(prefix = "app.query-understanding")
public class QueryUnderstandingProperties {

    private boolean enabled = true;

    private boolean rewriteEnabled = true;

    private boolean clarificationEnabled = true;

    private double minConfidence = 0.55;

    private int maxGlobalSearchDocuments = 10000;

    private int maxGlobalSearchCollections = 8;
}
