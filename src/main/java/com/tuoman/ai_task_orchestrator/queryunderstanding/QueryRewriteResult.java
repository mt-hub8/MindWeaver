package com.tuoman.ai_task_orchestrator.queryunderstanding;

import com.fasterxml.jackson.annotation.JsonInclude;
import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
@JsonInclude(JsonInclude.Include.NON_NULL)
public class QueryRewriteResult {

    private final String originalQuery;

    private final String normalizedQuery;

    private final String keywordQuery;

    private final String semanticQuery;

    private final String symbolQuery;

    private final String versionAwareQuery;
}
