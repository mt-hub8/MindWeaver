package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.RetrievalMetricAtKResponse;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.within;

class RetrievalMetricsCalculatorTest {

    private final RetrievalMetricsCalculator calculator = new RetrievalMetricsCalculator();

    @Test
    void calculateShouldComputeRecallHitRateMrrAndContextPrecisionForMultipleK() {
        List<RetrievalMetricAtKResponse> metrics = calculator.calculate(
                List.of(12L, 13L),
                List.of(8L, 12L, 20L, 13L, 30L),
                List.of(1, 3, 5)
        );

        assertMetric(metrics.get(0), 1, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0);
        assertMetric(metrics.get(1), 3, 0.5, 1.0 / 3.0, 1.0, 0.5, 0.386852, 1.0 / 3.0);
        assertMetric(metrics.get(2), 5, 1.0, 0.4, 1.0, 0.5, 0.650921, 0.4);
    }

    @Test
    void calculateShouldReturnZeroMetricsWhenNoRelevantChunkIsRetrieved() {
        List<RetrievalMetricAtKResponse> metrics = calculator.calculate(
                List.of(12L, 13L),
                List.of(8L, 20L, 30L),
                List.of(3)
        );

        assertMetric(metrics.getFirst(), 3, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0);
    }

    @Test
    void calculateShouldUseActualReturnedCountForContextPrecisionWhenReturnedLessThanK() {
        List<RetrievalMetricAtKResponse> metrics = calculator.calculate(
                List.of(12L, 13L),
                List.of(12L),
                List.of(5)
        );

        assertMetric(metrics.getFirst(), 5, 0.5, 0.2, 1.0, 1.0, 0.613147, 1.0);
        assertThat(metrics.getFirst().getPrecisionAtK())
                .isNotEqualTo(metrics.getFirst().getContextPrecisionAtK());
    }

    @Test
    void calculateShouldHandleEmptyRetrievedChunks() {
        List<RetrievalMetricAtKResponse> metrics = calculator.calculate(
                List.of(12L),
                List.of(),
                List.of(1, 5)
        );

        assertMetric(metrics.get(0), 1, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0);
        assertMetric(metrics.get(1), 5, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0);
    }

    @Test
    void calculateShouldDeduplicateExpectedChunkIds() {
        List<RetrievalMetricAtKResponse> metrics = calculator.calculate(
                List.of(12L, 12L, 13L),
                List.of(12L, 20L, 13L),
                List.of(3)
        );

        assertMetric(metrics.getFirst(), 3, 1.0, 2.0 / 3.0, 1.0, 1.0, 0.919721, 2.0 / 3.0);
    }

    @Test
    void calculateShouldReturnZeroMetricsWhenExpectedChunkIdsAreEmpty() {
        List<RetrievalMetricAtKResponse> metrics = calculator.calculate(
                List.of(),
                List.of(12L),
                List.of(1)
        );

        assertMetric(metrics.getFirst(), 1, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0);
    }

    @Test
    void calculateShouldReturnNdcgOneForIdealRanking() {
        List<RetrievalMetricAtKResponse> metrics = calculator.calculate(
                List.of(12L, 13L),
                List.of(12L, 13L, 30L),
                List.of(2)
        );

        assertThat(metrics.getFirst().getNdcgAtK()).isCloseTo(1.0, within(0.000001));
    }

    @Test
    void calculateShouldGiveHigherNdcgWhenRelevantChunksRankEarlier() {
        double earlyNdcg = calculator.calculate(
                List.of(12L, 13L),
                List.of(12L, 20L, 13L),
                List.of(3)
        ).getFirst().getNdcgAtK();
        double lateNdcg = calculator.calculate(
                List.of(12L, 13L),
                List.of(20L, 13L, 12L),
                List.of(3)
        ).getFirst().getNdcgAtK();

        assertThat(earlyNdcg).isGreaterThan(lateNdcg);
    }

    @Test
    void calculateShouldComputeNdcgWhenKIsSmallerThanExpectedRelevantCount() {
        List<RetrievalMetricAtKResponse> metrics = calculator.calculate(
                List.of(12L, 13L, 14L),
                List.of(12L, 20L),
                List.of(2)
        );

        assertThat(metrics.getFirst().getNdcgAtK()).isCloseTo(0.613147, within(0.000001));
    }

    private void assertMetric(
            RetrievalMetricAtKResponse metric,
            int k,
            double recallAtK,
            double precisionAtK,
            double hitRateAtK,
            double mrr,
            double ndcgAtK,
            double contextPrecisionAtK
    ) {
        assertThat(metric.getK()).isEqualTo(k);
        assertThat(metric.getRecallAtK()).isCloseTo(recallAtK, within(0.000001));
        assertThat(metric.getPrecisionAtK()).isCloseTo(precisionAtK, within(0.000001));
        assertThat(metric.getHitRateAtK()).isCloseTo(hitRateAtK, within(0.000001));
        assertThat(metric.getMrr()).isCloseTo(mrr, within(0.000001));
        assertThat(metric.getNdcgAtK()).isCloseTo(ndcgAtK, within(0.000001));
        assertThat(metric.getContextPrecisionAtK()).isCloseTo(contextPrecisionAtK, within(0.000001));
    }
}
