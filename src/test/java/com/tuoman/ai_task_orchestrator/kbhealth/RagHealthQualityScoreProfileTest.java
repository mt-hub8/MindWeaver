package com.tuoman.ai_task_orchestrator.kbhealth;

import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class RagHealthQualityScoreProfileTest {

    private final RagHealthQualityScoreCalculator calculator = new RagHealthQualityScoreCalculator();

    @Test
    void balancedProfileShouldWeightMetrics() {
        List<HealthMetricValue> metrics = baseMetrics();
        var result = calculator.score(metrics, RagHealthScoringProfile.BALANCED);
        assertThat(result.getOverallScore()).isBetween(0, 100);
        assertThat(result.getBreakdown()).isNotEmpty();
    }

    @Test
    void preciseProfileShouldPreferPrecisionAndIsolation() {
        var result = calculator.score(baseMetrics(), RagHealthScoringProfile.PRECISE);
        assertThat(result.getProfile()).isEqualTo(RagHealthScoringProfile.PRECISE);
    }

    @Test
    void comprehensiveProfileShouldPreferRecallAndNdcg() {
        var result = calculator.score(baseMetrics(), RagHealthScoringProfile.COMPREHENSIVE);
        assertThat(result.getProfile()).isEqualTo(RagHealthScoringProfile.COMPREHENSIVE);
    }

    @Test
    void generationTrustProfileShouldIncludeFaithfulness() {
        var result = calculator.score(baseMetrics(), RagHealthScoringProfile.GENERATION_TRUST);
        assertThat(result.getProfile()).isEqualTo(RagHealthScoringProfile.GENERATION_TRUST);
    }

    @Test
    void leakRateShouldInvertForScoring() {
        List<HealthMetricValue> metrics = List.of(
                HealthMetricValue.of("RECALL_AT_K", "Recall", 0.8, false),
                HealthMetricValue.of("CONTEXT_PRECISION_AT_K", "Precision", 0.8, false),
                HealthMetricValue.of("MRR_AT_K", "MRR", 0.8, false),
                HealthMetricValue.of("NDCG_AT_K", "NDCG", 0.8, false),
                HealthMetricValue.ofLeak("CROSS_COLLECTION_LEAK_RATE", "Cross", 0.2),
                HealthMetricValue.ofLeak("WRONG_VERSION_LEAK_RATE", "Version", 0.1),
                HealthMetricValue.of("FAITHFULNESS", "Faith", 0.8, true),
                HealthMetricValue.of("CITATION_ACCURACY", "Citation", 0.8, false)
        );
        var balanced = calculator.score(metrics, RagHealthScoringProfile.BALANCED);
        assertThat(balanced.getOverallScore()).isGreaterThan(60);
    }

    @Test
    void unavailableMetricsShouldNotBreakScore() {
        List<HealthMetricValue> metrics = List.of(
                HealthMetricValue.unavailable("NDCG_AT_K", "NDCG", "missing"),
                HealthMetricValue.of("RECALL_AT_K", "Recall", 0.9, false),
                HealthMetricValue.of("CONTEXT_PRECISION_AT_K", "Precision", 0.9, false),
                HealthMetricValue.of("MRR_AT_K", "MRR", 0.9, false),
                HealthMetricValue.ofLeak("CROSS_COLLECTION_LEAK_RATE", "Cross", 0.0),
                HealthMetricValue.ofLeak("WRONG_VERSION_LEAK_RATE", "Version", 0.0),
                HealthMetricValue.of("FAITHFULNESS", "Faith", 0.9, true),
                HealthMetricValue.of("CITATION_ACCURACY", "Citation", 0.9, false)
        );
        var result = calculator.score(metrics, RagHealthScoringProfile.BALANCED);
        assertThat(result.getOverallScore()).isBetween(0, 100);
    }

    private List<HealthMetricValue> baseMetrics() {
        return List.of(
                HealthMetricValue.of("RECALL_AT_K", "Recall", 0.7, false),
                HealthMetricValue.of("CONTEXT_PRECISION_AT_K", "Precision", 0.7, false),
                HealthMetricValue.of("MRR_AT_K", "MRR", 0.7, false),
                HealthMetricValue.of("NDCG_AT_K", "NDCG", 0.7, false),
                HealthMetricValue.ofLeak("CROSS_COLLECTION_LEAK_RATE", "Cross", 0.1),
                HealthMetricValue.ofLeak("WRONG_VERSION_LEAK_RATE", "Version", 0.05),
                HealthMetricValue.of("FAITHFULNESS", "Faith", 0.7, true),
                HealthMetricValue.of("CITATION_ACCURACY", "Citation", 0.7, false),
                HealthMetricValue.of("ANSWER_COVERAGE", "Coverage", 0.7, false),
                HealthMetricValue.of("REFUSAL_ACCURACY", "Refusal", 0.7, false)
        );
    }
}
