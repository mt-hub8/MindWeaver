package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.config.ChunkingProperties;
import com.tuoman.ai_task_orchestrator.config.RetrievalPipelineProperties;
import com.tuoman.ai_task_orchestrator.dto.RetrievalSettingsResponse;
import com.tuoman.ai_task_orchestrator.hybrid.RagHybridProperties;
import com.tuoman.ai_task_orchestrator.queryunderstanding.QueryUnderstandingProperties;
import com.tuoman.ai_task_orchestrator.rerank.RagRerankProperties;
import lombok.RequiredArgsConstructor;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/retrieval/settings")
@RequiredArgsConstructor
public class RetrievalSettingsController {

    private final ChunkingProperties chunkingProperties;

    private final RetrievalPipelineProperties pipelineProperties;

    private final RagHybridProperties ragHybridProperties;

    private final RagRerankProperties ragRerankProperties;

    private final QueryUnderstandingProperties queryUnderstandingProperties;

    @GetMapping
    public RetrievalSettingsResponse getSettings() {
        return new RetrievalSettingsResponse(
                chunkingProperties.getStrategy() == null ? null : chunkingProperties.getStrategy().name(),
                chunkingProperties.getMaxChars(),
                chunkingProperties.getOverlapChars(),
                chunkingProperties.isPreserveCodeBlock(),
                chunkingProperties.isIncludeSectionPath(),
                ragHybridProperties.isEnabled() || pipelineProperties.isHybridEnabled(),
                ragRerankProperties.isEnabled() || pipelineProperties.isRerankEnabled(),
                pipelineProperties.getContextExpansion() == null ? null : pipelineProperties.getContextExpansion().name(),
                pipelineProperties.getDefaultFusion() == null ? null : pipelineProperties.getDefaultFusion().name(),
                pipelineProperties.getRrfK(),
                ragRerankProperties.getProvider(),
                "APPLICATION_SIDE",
                queryUnderstandingProperties.isEnabled(),
                queryUnderstandingProperties.isRewriteEnabled(),
                queryUnderstandingProperties.isClarificationEnabled(),
                queryUnderstandingProperties.getMinConfidence(),
                queryUnderstandingProperties.getMaxGlobalSearchDocuments(),
                "heuristic-rule-based"
        );
    }
}
