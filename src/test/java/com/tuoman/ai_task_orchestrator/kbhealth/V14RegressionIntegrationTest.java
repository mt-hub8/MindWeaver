package com.tuoman.ai_task_orchestrator.kbhealth;

import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class V14RegressionIntegrationTest {

    @Test
    void hybridRrfStrategyShouldRemainAvailable() {
        assertThat(RagEvaluationRetrievalStrategy.HYBRID_RRF).isNotNull();
        assertThat(RagEvaluationRetrievalStrategy.HYBRID_RRF_RERANK_PARENT_CONTEXT).isNotNull();
    }

    @Test
    void crossCollectionMetricCalculatorShouldStillWork() {
        var calculator = new RagHealthRetrievalMetricsCalculator();
        var evalCase = new com.tuoman.ai_task_orchestrator.entity.RagEvaluationCaseEntity();
        evalCase.setCollectionId(1L);
        var metrics = calculator.calculate(evalCase, List.of(), 5);
        assertThat(metrics.stream().anyMatch(m -> "CROSS_COLLECTION_LEAK_RATE".equals(m.getCode()))).isTrue();
    }
}
