package com.tuoman.ai_task_orchestrator.config;

import com.tuoman.ai_task_orchestrator.enums.ContextExpansionStrategy;
import com.tuoman.ai_task_orchestrator.enums.RetrievalFusionStrategy;
import lombok.Getter;
import lombok.Setter;
import org.springframework.boot.context.properties.ConfigurationProperties;
import org.springframework.stereotype.Component;

@Getter
@Setter
@Component
@ConfigurationProperties(prefix = "app.retrieval")
public class RetrievalPipelineProperties {

    private ContextExpansionStrategy contextExpansion = ContextExpansionStrategy.ADJACENT;

    private int maxExpandedChunks = 2;

    private int maxContextChars = 6000;

    private RetrievalFusionStrategy defaultFusion = RetrievalFusionStrategy.RRF;

    private String rerankerMode = "heuristic";

    private boolean hybridEnabled = false;

    private boolean rerankEnabled = false;

    private int vectorTopK = 20;

    private int keywordTopK = 20;

    private int finalTopK = 5;

    private int rrfK = 60;
}
