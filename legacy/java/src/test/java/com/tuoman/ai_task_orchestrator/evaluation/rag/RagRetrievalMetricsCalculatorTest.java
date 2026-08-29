package com.tuoman.ai_task_orchestrator.evaluation.rag;

import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.within;

class RagRetrievalMetricsCalculatorTest {

    private final RagRetrievalMetricsCalculator calculator = new RagRetrievalMetricsCalculator();

    @Test
    void calculateShouldComputeHitRecallPrecisionAndRr() {
        List<RagRetrievalExpectedItem> expected = List.of(
                item("e1", "chunkHash"),
                item("e2", "provider")
        );
        List<RagRetrievedItem> retrieved = List.of(
                retrieved(1, 101L, "irrelevant"),
                retrieved(2, 102L, "chunkHash provider model dimension"),
                retrieved(3, 103L, "provider")
        );
        List<RagRetrievalExpectedItem> matched = List.of(expected.get(0), expected.get(1));

        RagCaseMetrics metrics = calculator.calculate(expected, matched, retrieved);

        assertThat(metrics.hit()).isTrue();
        assertThat(metrics.recallAtK()).isCloseTo(1.0, within(0.000001));
        assertThat(metrics.precisionAtK()).isCloseTo(2.0 / 3.0, within(0.000001));
        assertThat(metrics.rrAtK()).isCloseTo(0.5, within(0.000001));
    }

    @Test
    void calculateShouldReturnZeroMetricsWhenNoRelevantResult() {
        List<RagRetrievalExpectedItem> expected = List.of(item("e1", "cache key"));
        List<RagRetrievedItem> retrieved = List.of(retrieved(1, 101L, "unrelated"));

        RagCaseMetrics metrics = calculator.calculate(expected, List.of(), retrieved);

        assertThat(metrics.hit()).isFalse();
        assertThat(metrics.recallAtK()).isZero();
        assertThat(metrics.precisionAtK()).isZero();
        assertThat(metrics.rrAtK()).isZero();
    }

    @Test
    void matchesShouldSupportChunkContainsAndDocumentId() {
        RagRetrievalExpectedItem expected = new RagRetrievalExpectedItem(
                "e1",
                1L,
                null,
                null,
                "RAG Answer API"
        );
        RagRetrievedItem matched = new RagRetrievedItem(1, 1L, "RAG", 10L, 0.9, "RAG Answer API core flow");
        RagRetrievedItem unmatched = new RagRetrievedItem(1, 2L, "Other", 11L, 0.8, "RAG Answer API core flow");

        assertThat(calculator.matches(expected, matched)).isTrue();
        assertThat(calculator.matches(expected, unmatched)).isFalse();
    }

    private RagRetrievalExpectedItem item(String expectedId, String chunkContains) {
        return new RagRetrievalExpectedItem(expectedId, null, null, null, chunkContains);
    }

    private RagRetrievedItem retrieved(int rank, Long chunkId, String content) {
        return new RagRetrievedItem(rank, 1L, "heading", chunkId, 0.9, content);
    }
}
