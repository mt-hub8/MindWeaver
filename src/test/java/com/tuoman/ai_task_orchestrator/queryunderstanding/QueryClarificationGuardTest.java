package com.tuoman.ai_task_orchestrator.queryunderstanding;

import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class QueryClarificationGuardTest {

    private final QueryClarificationGuard guard = new QueryClarificationGuard(new QueryUnderstandingProperties());

    @Test
    void shouldClarifyShortLowConfidenceGlobalQueries() {
        var result = QueryUnderstandingResult.builder()
                .originalQuery("啥")
                .normalizedQuery("啥")
                .queryType(QueryType.AMBIGUOUS)
                .confidence(0.3)
                .entities(List.of())
                .codeSymbols(List.of())
                .configKeys(List.of())
                .apiPaths(List.of())
                .build();

        var guarded = guard.evaluate(result, null, null, 20000, 20);

        assertThat(guarded.clarificationRequired()).isTrue();
        assertThat(guarded.clarificationQuestion()).contains("当前问题比较模糊");
    }

    @Test
    void selectedCollectionShouldAvoidClarification() {
        var result = QueryUnderstandingResult.builder()
                .originalQuery("啥")
                .queryType(QueryType.AMBIGUOUS)
                .confidence(0.3)
                .entities(List.of())
                .codeSymbols(List.of())
                .configKeys(List.of())
                .apiPaths(List.of())
                .build();

        var guarded = guard.evaluate(result, null, UserSelectedFilters.ofCollection(1L), 20000, 20);

        assertThat(guarded.clarificationRequired()).isFalse();
    }
}
