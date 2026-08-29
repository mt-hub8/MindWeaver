package com.tuoman.ai_task_orchestrator.hybrid;

import lombok.Getter;
import lombok.Setter;
import org.springframework.boot.context.properties.ConfigurationProperties;

@Getter
@Setter
@ConfigurationProperties(prefix = "rag.hybrid")
public class RagHybridProperties {

    private boolean enabled = false;

    private int denseTopK = 20;

    private int lexicalTopK = 20;

    private int finalTopK = 5;

    private String fusion = RrfFusionRanker.STRATEGY;

    private int rrfK = 60;
}
