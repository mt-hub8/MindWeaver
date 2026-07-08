package com.tuoman.ai_task_orchestrator.queryunderstanding;

import com.fasterxml.jackson.annotation.JsonInclude;
import com.tuoman.ai_task_orchestrator.enums.ChunkMetadataStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentDocType;
import lombok.Builder;
import lombok.Getter;

import java.util.List;

@Getter
@Builder(toBuilder = true)
@JsonInclude(JsonInclude.Include.NON_NULL)
public class QueryUnderstandingResult {

    private final String originalQuery;

    private final String normalizedQuery;

    private final QueryType queryType;

    private final double confidence;

    private final Long collectionHint;

    private final String versionHint;

    private final DocumentDocType docTypeHint;

    private final String sourceHint;

    private final ChunkMetadataStatus statusHint;

    private final List<String> tags;

    private final List<String> entities;

    private final List<String> codeSymbols;

    private final List<String> configKeys;

    private final List<String> apiPaths;

    private final List<String> timeHints;

    private final boolean requiresHybrid;

    private final boolean requiresRerank;

    private final boolean requiresContextExpansion;

    private final boolean requiresClarification;

    private final boolean noAnswerRisk;

    private final List<String> warnings;

    private final List<String> reasons;

    public boolean hasEntities() {
        return (entities != null && !entities.isEmpty())
                || (codeSymbols != null && !codeSymbols.isEmpty())
                || (configKeys != null && !configKeys.isEmpty())
                || (apiPaths != null && !apiPaths.isEmpty())
                || versionHint != null
                || docTypeHint != null;
    }
}
