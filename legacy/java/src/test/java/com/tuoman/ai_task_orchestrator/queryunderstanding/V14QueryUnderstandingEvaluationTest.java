package com.tuoman.ai_task_orchestrator.queryunderstanding;

import com.tuoman.ai_task_orchestrator.entity.RagEvaluationCaseEntity;
import com.tuoman.ai_task_orchestrator.kbhealth.HealthMetricValue;
import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationQueryType;
import com.tuoman.ai_task_orchestrator.retrieval.RetrievalFilter;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class V14QueryUnderstandingEvaluationTest {

    private final QueryUnderstandingMetricsService service = new QueryUnderstandingMetricsService();

    @Test
    void shouldCalculateCollectionRoutingAccuracyWhenExpectedExists() {
        RagEvaluationCaseEntity evalCase = new RagEvaluationCaseEntity();
        evalCase.setQueryType(RagEvaluationQueryType.VERSION_SPECIFIC);
        evalCase.setCollectionId(3L);
        evalCase.setMetadataFilterJson("{\"version\":\"V10.0\"}");

        QueryUnderstandingResult understanding = QueryUnderstandingResult.builder()
                .queryType(QueryType.VERSION_SPECIFIC)
                .versionHint("V10.0")
                .build();
        RetrievalRoutingDecision decision = RetrievalRoutingDecision.builder()
                .filter(RetrievalFilter.builder().collectionId(3L).version("V10.0").build())
                .build();

        List<HealthMetricValue> metrics = service.calculate(evalCase, understanding, decision);

        assertThat(metric(metrics, "COLLECTION_ROUTING_ACCURACY").getRawValue()).isEqualTo(1.0);
        assertThat(metric(metrics, "VERSION_EXTRACTION_ACCURACY").getRawValue()).isEqualTo(1.0);
    }

    @Test
    void missingExpectedShouldRemainUnknown() {
        RagEvaluationCaseEntity evalCase = new RagEvaluationCaseEntity();
        evalCase.setMetadataFilterJson("{}");

        List<HealthMetricValue> metrics = service.calculate(
                evalCase,
                QueryUnderstandingResult.builder().queryType(QueryType.SINGLE_DOC_FACT).build(),
                RetrievalRoutingDecision.builder().filter(RetrievalFilter.empty()).build()
        );

        assertThat(metric(metrics, "COLLECTION_ROUTING_ACCURACY").isAvailable()).isFalse();
        assertThat(metric(metrics, "FILTER_EXTRACTION_ACCURACY").isAvailable()).isFalse();
    }

    private HealthMetricValue metric(List<HealthMetricValue> metrics, String code) {
        return metrics.stream().filter(m -> code.equals(m.getCode())).findFirst().orElseThrow();
    }
}
