package com.tuoman.ai_task_orchestrator.rag.quality;

import com.tuoman.ai_task_orchestrator.dto.RagCitationResponse;
import com.tuoman.ai_task_orchestrator.dto.RagGenerationMetadataResponse;
import com.tuoman.ai_task_orchestrator.dto.RagRetrievalMetadataResponse;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatCode;

class RagQualityScoreCalculatorTest {

    private RagQualityScoreCalculator calculator;

    @BeforeEach
    void setUp() {
        calculator = new RagQualityScoreCalculator();
    }

    @Test
    void balancedModeShouldUseExpectedWeights() {
        RagQualityWeights weights = RagQualityWeights.forMode(RagQualityMode.BALANCED);
        assertThat(weights.getRetrieval()).isEqualTo(0.30);
        assertThat(weights.getContext()).isEqualTo(0.25);
        assertThat(weights.getAnswer()).isEqualTo(0.25);
        assertThat(weights.getCitation()).isEqualTo(0.20);
    }

    @Test
    void preciseModeShouldUseExpectedWeights() {
        RagQualityWeights weights = RagQualityWeights.forMode(RagQualityMode.PRECISE);
        assertThat(weights.getRetrieval()).isEqualTo(0.20);
        assertThat(weights.getContext()).isEqualTo(0.30);
        assertThat(weights.getAnswer()).isEqualTo(0.30);
        assertThat(weights.getCitation()).isEqualTo(0.20);
    }

    @Test
    void comprehensiveModeShouldUseExpectedWeights() {
        RagQualityWeights weights = RagQualityWeights.forMode(RagQualityMode.COMPREHENSIVE);
        assertThat(weights.getRetrieval()).isEqualTo(0.40);
        assertThat(weights.getContext()).isEqualTo(0.20);
        assertThat(weights.getAnswer()).isEqualTo(0.20);
        assertThat(weights.getCitation()).isEqualTo(0.20);
    }

    @Test
    void overallScoreShouldBeWeightedSumRoundedToInteger() {
        RagQualityScoreContext context = RagQualityScoreContext.builder()
                .query("Java Python 分工")
                .answer("Java 负责编排，Python 负责 worker。")
                .citations(List.of(citation(1L, 10L, 0.92, "Java Spring Boot 编排")))
                .retrieval(retrieval(5, 1, 1))
                .generation(generation(false, null, 1200L))
                .mode(RagQualityMode.BALANCED)
                .build();

        RagQualityScoreResult result = calculator.calculate(context);
        int expected = (int) Math.round(
                result.getRetrievalScore() * 0.30
                        + result.getContextScore() * 0.25
                        + result.getAnswerScore() * 0.25
                        + result.getCitationScore() * 0.20
        );
        assertThat(result.getOverallScore()).isEqualTo(expected);
    }

    @Test
    void missingLabeledMetricsShouldNotThrow() {
        RagQualityScoreContext context = RagQualityScoreContext.builder()
                .query("test")
                .answer("answer")
                .citations(List.of())
                .retrieval(retrieval(5, 0, 0))
                .generation(generation(true, "NO_RETRIEVED_CONTEXT", null))
                .mode(RagQualityMode.BALANCED)
                .build();

        assertThatCode(() -> calculator.calculate(context)).doesNotThrowAnyException();
        RagQualityScoreResult result = calculator.calculate(context);
        assertThat(result.getScoringNote()).contains("启发式");
    }

    @Test
    void noContextShouldProduceLowScores() {
        RagQualityScoreContext context = RagQualityScoreContext.builder()
                .query("unknown topic")
                .answer("根据当前检索到的文档内容，无法确定。")
                .citations(List.of())
                .retrieval(retrieval(5, 0, 0))
                .generation(generation(true, "NO_RETRIEVED_CONTEXT", null))
                .mode(RagQualityMode.BALANCED)
                .build();

        RagQualityScoreResult result = calculator.calculate(context);
        assertThat(result.getRetrievalScore()).isLessThan(40);
        assertThat(result.getOverallScore()).isLessThan(50);
    }

    @Test
    void scoresShouldStayWithinZeroToOneHundred() {
        RagQualityScoreContext context = RagQualityScoreContext.builder()
                .query("embedding cache key")
                .answer("Embedding Cache 使用 provider model chunk hash 作为 cache key。")
                .citations(List.of(
                        citation(1L, 11L, 0.99, "cache key provider model chunk hash"),
                        citation(2L, 12L, 0.98, "embedding cache metrics")
                ))
                .retrieval(retrieval(5, 2, 2))
                .generation(generation(false, null, 500L))
                .mode(RagQualityMode.COMPREHENSIVE)
                .recallAtK(1.2)
                .hitRateAtK(1.0)
                .build();

        RagQualityScoreResult result = calculator.calculate(context);
        assertThat(result.getRetrievalScore()).isBetween(0, 100);
        assertThat(result.getContextScore()).isBetween(0, 100);
        assertThat(result.getAnswerScore()).isBetween(0, 100);
        assertThat(result.getCitationScore()).isBetween(0, 100);
        assertThat(result.getOverallScore()).isBetween(0, 100);
    }

    private RagRetrievalMetadataResponse retrieval(int topK, int returned, int finalContextCount) {
        return new RagRetrievalMetadataResponse(
                topK,
                returned,
                "mock",
                "mock-embedding-v1",
                128,
                "ExactCosineVectorStore",
                null,
                null,
                null,
                topK,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                null,
                "ALL_DOCUMENTS",
                null,
                null,
                null,
                null,
                finalContextCount
        );
    }

    private RagGenerationMetadataResponse generation(boolean skipped, String reason, Long latencyMs) {
        return new RagGenerationMetadataResponse(
                skipped ? null : "mock",
                skipped ? null : "mock-llm",
                skipped ? null : "mock",
                skipped ? null : "mock-llm",
                skipped,
                reason,
                latencyMs,
                null,
                null
        );
    }

    private RagCitationResponse citation(Long documentId, Long chunkId, double score, String snippet) {
        return new RagCitationResponse(1, documentId, chunkId, score, snippet);
    }
}
