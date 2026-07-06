package com.tuoman.ai_task_orchestrator.config;

import com.tuoman.ai_task_orchestrator.enums.ChunkingStrategy;
import com.tuoman.ai_task_orchestrator.enums.ContextExpansionStrategy;
import com.tuoman.ai_task_orchestrator.enums.RetrievalFusionStrategy;
import lombok.Getter;
import lombok.Setter;
import org.springframework.boot.context.properties.ConfigurationProperties;
import org.springframework.stereotype.Component;

@Getter
@Setter
@Component
@ConfigurationProperties(prefix = "app.chunking")
public class ChunkingProperties {

    private ChunkingStrategy strategy = ChunkingStrategy.STRUCTURE_AWARE_WITH_OVERLAP;

    private int maxChars = 1200;

    private int minChars = 200;

    private int overlapChars = 120;

    private boolean preserveCodeBlock = true;

    private boolean preserveTableBlock = true;

    private boolean includeSectionPath = true;

    public boolean isStrategyWithOverlap() {
        return strategy == ChunkingStrategy.STRUCTURE_AWARE_WITH_OVERLAP || overlapChars > 0;
    }
}
