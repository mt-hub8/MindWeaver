package com.tuoman.ai_task_orchestrator.queryunderstanding;

import com.fasterxml.jackson.annotation.JsonInclude;
import lombok.Builder;
import lombok.Getter;

import java.util.List;

@Getter
@Builder
@JsonInclude(JsonInclude.Include.NON_NULL)
public class QueryUnderstandingDiagnostics {

    private final String query;

    private final QueryType queryType;

    private final double confidence;

    private final List<String> extractedEntities;

    private final String extractedVersion;

    private final String extractedDocType;

    private final Long extractedCollection;

    private final List<String> extractedSymbols;

    private final List<String> extractedConfigKeys;

    private final List<String> extractedApiPaths;

    private final QueryRewriteResult rewrittenQueries;

    private final RetrievalRoutingDecision routingDecision;

    private final boolean clarificationRequired;

    private final String clarificationReason;

    private final List<String> warnings;

    public static QueryUnderstandingDiagnostics from(
            QueryUnderstandingResult understanding,
            QueryRewriteResult rewrite,
            RetrievalRoutingDecision decision,
            String clarificationReason
    ) {
        return QueryUnderstandingDiagnostics.builder()
                .query(understanding.getOriginalQuery())
                .queryType(understanding.getQueryType())
                .confidence(understanding.getConfidence())
                .extractedEntities(understanding.getEntities())
                .extractedVersion(understanding.getVersionHint())
                .extractedDocType(understanding.getDocTypeHint() == null ? null : understanding.getDocTypeHint().name())
                .extractedCollection(understanding.getCollectionHint())
                .extractedSymbols(understanding.getCodeSymbols())
                .extractedConfigKeys(understanding.getConfigKeys())
                .extractedApiPaths(understanding.getApiPaths())
                .rewrittenQueries(rewrite)
                .routingDecision(decision)
                .clarificationRequired(decision != null && decision.isClarificationRequired())
                .clarificationReason(clarificationReason)
                .warnings(understanding.getWarnings())
                .build();
    }
}
