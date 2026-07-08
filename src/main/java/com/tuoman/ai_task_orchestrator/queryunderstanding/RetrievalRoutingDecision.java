package com.tuoman.ai_task_orchestrator.queryunderstanding;

import com.fasterxml.jackson.annotation.JsonInclude;
import com.tuoman.ai_task_orchestrator.enums.ContextExpansionStrategy;
import com.tuoman.ai_task_orchestrator.retrieval.RetrievalFilter;
import lombok.Builder;
import lombok.Getter;

import java.util.List;

@Getter
@Builder
@JsonInclude(JsonInclude.Include.NON_NULL)
public class RetrievalRoutingDecision {

    private final RetrievalRoutingStrategy strategy;

    private final RetrievalFilter filter;

    private final int vectorTopK;

    private final int keywordTopK;

    private final int finalTopK;

    private final int rerankTopN;

    private final ContextExpansionStrategy contextExpansion;

    private final String scoringProfile;

    private final boolean clarificationRequired;

    private final String clarificationQuestion;

    private final List<String> routingReasons;

    private final List<String> warnings;

    private final boolean noAnswerRisk;

    public boolean rerankEnabled() {
        return strategy == RetrievalRoutingStrategy.HYBRID_RRF_RERANK
                || strategy == RetrievalRoutingStrategy.HYBRID_RRF_RERANK_PARENT_CONTEXT;
    }

    public boolean contextExpansionEnabled() {
        return contextExpansion != null && contextExpansion != ContextExpansionStrategy.NONE;
    }
}
