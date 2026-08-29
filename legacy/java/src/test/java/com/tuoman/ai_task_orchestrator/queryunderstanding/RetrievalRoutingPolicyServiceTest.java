package com.tuoman.ai_task_orchestrator.queryunderstanding;

import com.tuoman.ai_task_orchestrator.config.RetrievalPipelineProperties;
import com.tuoman.ai_task_orchestrator.enums.ChunkMetadataStatus;
import com.tuoman.ai_task_orchestrator.enums.ContextExpansionStrategy;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class RetrievalRoutingPolicyServiceTest {

    private RetrievalRoutingPolicyService service;

    @BeforeEach
    void setUp() {
        service = new RetrievalRoutingPolicyService(new RetrievalPipelineProperties());
    }

    @Test
    void shouldRouteByQueryType() {
        assertThat(route(QueryType.CODE_SYMBOL).getStrategy()).isEqualTo(RetrievalRoutingStrategy.HYBRID_RRF_RERANK);

        var version = base(QueryType.VERSION_SPECIFIC).toBuilder().versionHint("V10.0").build();
        assertThat(service.route(version, null).getFilter().getVersion()).isEqualTo("V10.0");

        var latest = service.route(base(QueryType.LATEST_VERSION), null);
        assertThat(latest.getFilter().getStatus()).isEqualTo(ChunkMetadataStatus.ACTIVE);
        assertThat(latest.getFilter().isIncludeDeprecated()).isFalse();

        assertThat(route(QueryType.SUMMARY).getContextExpansion()).isEqualTo(ContextExpansionStrategy.PARENT_AND_ADJACENT);
        assertThat(route(QueryType.AMBIGUOUS).isClarificationRequired()).isTrue();
        assertThat(route(QueryType.NO_ANSWER_RISK).isNoAnswerRisk()).isTrue();
    }

    private RetrievalRoutingDecision route(QueryType type) {
        return service.route(base(type), null);
    }

    private QueryUnderstandingResult base(QueryType type) {
        return QueryUnderstandingResult.builder()
                .originalQuery("query")
                .normalizedQuery("query")
                .queryType(type)
                .confidence(type == QueryType.AMBIGUOUS ? 0.3 : 0.8)
                .entities(List.of("query"))
                .codeSymbols(List.of())
                .configKeys(List.of())
                .apiPaths(List.of())
                .tags(List.of())
                .warnings(List.of())
                .reasons(List.of())
                .build();
    }
}
