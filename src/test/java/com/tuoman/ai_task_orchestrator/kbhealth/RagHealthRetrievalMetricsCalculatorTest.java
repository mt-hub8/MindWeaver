package com.tuoman.ai_task_orchestrator.kbhealth;

import com.tuoman.ai_task_orchestrator.entity.RagEvaluationCaseEntity;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class RagHealthRetrievalMetricsCalculatorTest {

    private final RagHealthRetrievalMetricsCalculator calculator = new RagHealthRetrievalMetricsCalculator();

    @Test
    void recallHitMrrNdcgAndPrecisionShouldComputeCorrectly() {
        RagEvaluationCaseEntity evalCase = caseWithExpectedChunks(1L, 2L);
        List<EvaluationRetrievedChunk> retrieved = List.of(
                chunk(10L, 100L),
                chunk(1L, 101L),
                chunk(2L, 102L)
        );

        List<HealthMetricValue> metrics = calculator.calculate(evalCase, retrieved, 3);

        assertThat(metric(metrics, "RECALL_AT_K").getRawValue()).isEqualTo(1.0);
        assertThat(metric(metrics, "HIT_RATE_AT_K").getRawValue()).isEqualTo(1.0);
        assertThat(metric(metrics, "MRR_AT_K").getRawValue()).isEqualTo(0.5);
        assertThat(metric(metrics, "NDCG_AT_K").getRawValue()).isGreaterThan(0.0);
        assertThat(metric(metrics, "CONTEXT_PRECISION_AT_K").getRawValue()).isEqualTo(2.0 / 3.0);
    }

    @Test
    void crossCollectionAndWrongVersionLeakShouldCompute() {
        RagEvaluationCaseEntity evalCase = caseWithExpectedChunks(1L);
        evalCase.setCollectionId(5L);
        evalCase.setMetadataFilterJson("{\"version\":\"V10.0\"}");

        List<EvaluationRetrievedChunk> retrieved = List.of(
                new EvaluationRetrievedChunk(1L, 1L, 5L, null, "V10.0", null, null, "a", 0.9, 1,
                        RagEvaluationRetrievalSource.VECTOR, null, true, false, false, false),
                new EvaluationRetrievedChunk(2L, 2L, 9L, null, "V9.0", null, null, "b", 0.8, 2,
                        RagEvaluationRetrievalSource.VECTOR, null, false, false, true, true)
        );

        List<HealthMetricValue> metrics = calculator.calculate(evalCase, retrieved, 2);

        assertThat(metric(metrics, "CROSS_COLLECTION_LEAK_RATE").getRawValue()).isEqualTo(0.5);
        assertThat(metric(metrics, "WRONG_VERSION_LEAK_RATE").getRawValue()).isEqualTo(0.5);
    }

    @Test
    void missingExpectedIdsShouldMarkMetricsUnavailable() {
        RagEvaluationCaseEntity evalCase = new RagEvaluationCaseEntity();
        evalCase.setQuery("test");

        List<HealthMetricValue> metrics = calculator.calculate(evalCase, List.of(chunk(1L, 1L)), 5);

        assertThat(metric(metrics, "RECALL_AT_K").isAvailable()).isFalse();
        assertThat(metric(metrics, "NDCG_AT_K").isAvailable()).isFalse();
        assertThat(metric(metrics, "CROSS_COLLECTION_LEAK_RATE").isAvailable()).isFalse();
    }

    private RagEvaluationCaseEntity caseWithExpectedChunks(Long... chunkIds) {
        RagEvaluationCaseEntity evalCase = new RagEvaluationCaseEntity();
        evalCase.setExpectedChunkIdsJson(JsonFieldCodec.write(List.of(chunkIds)));
        return evalCase;
    }

    private EvaluationRetrievedChunk chunk(Long chunkId, Long docId) {
        return new EvaluationRetrievedChunk(chunkId, docId, null, null, null, null, null, "text", 0.9, 1,
                RagEvaluationRetrievalSource.VECTOR, null, false, false, false, false);
    }

    private HealthMetricValue metric(List<HealthMetricValue> metrics, String code) {
        return metrics.stream().filter(m -> code.equals(m.getCode())).findFirst().orElseThrow();
    }
}
