package com.tuoman.ai_task_orchestrator.kbhealth;

import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class RagQualityVetoRuleServiceTest {

    private final RagQualityVetoRuleService service = new RagQualityVetoRuleService();

    @Test
    void crossCollectionLeakAbove30PercentShouldCapAt70() {
        var result = service.apply(90, List.of(HealthMetricValue.ofLeak("CROSS_COLLECTION_LEAK_RATE", "x", 0.31)));
        assertThat(result.getFinalScore()).isEqualTo(70);
    }

    @Test
    void wrongVersionLeakAbove25PercentShouldCapAt75() {
        var result = service.apply(90, List.of(HealthMetricValue.ofLeak("WRONG_VERSION_LEAK_RATE", "x", 0.26)));
        assertThat(result.getFinalScore()).isEqualTo(75);
    }

    @Test
    void faithfulnessBelowHalfShouldCapAt65() {
        var result = service.apply(90, List.of(HealthMetricValue.of("FAITHFULNESS", "x", 0.4, true)));
        assertThat(result.getFinalScore()).isEqualTo(65);
    }

    @Test
    void recallBelow40PercentShouldCapAt60() {
        var result = service.apply(90, List.of(HealthMetricValue.of("RECALL_AT_K", "x", 0.39, false)));
        assertThat(result.getFinalScore()).isEqualTo(60);
    }

    @Test
    void citationAccuracyBelowHalfShouldCapAt70() {
        var result = service.apply(90, List.of(HealthMetricValue.of("CITATION_ACCURACY", "x", 0.49, false)));
        assertThat(result.getFinalScore()).isEqualTo(70);
    }
}
