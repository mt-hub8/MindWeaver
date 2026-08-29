package com.tuoman.ai_task_orchestrator.kbhealth;

import com.tuoman.ai_task_orchestrator.entity.RagEvaluationCaseEntity;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class RagHealthGenerationMetricsCalculatorTest {

    private final RagHealthGenerationMetricsCalculator calculator = new RagHealthGenerationMetricsCalculator();

    @Test
    void answerCoverageShouldMatchKeywords() {
        RagEvaluationCaseEntity evalCase = new RagEvaluationCaseEntity();
        evalCase.setExpectedAnswerPointsJson("[\"支持多模型供应商\",\"API Key 管理\"]");

        List<HealthMetricValue> metrics = calculator.calculate(
                evalCase,
                "系统支持多模型供应商配置，并提供 API Key 管理。",
                List.of(),
                List.of()
        );

        assertThat(metric(metrics, "ANSWER_COVERAGE").getRawValue()).isEqualTo(1.0);
    }

    @Test
    void citationAccuracyShouldPenalizeMissingCitationsWhenRequired() {
        RagEvaluationCaseEntity evalCase = new RagEvaluationCaseEntity();
        evalCase.setExpectedChunkIdsJson("[1]");
        evalCase.setAnswerMustCite(true);

        List<HealthMetricValue> metrics = calculator.calculate(evalCase, "answer", List.of(), List.of());

        assertThat(metric(metrics, "CITATION_ACCURACY").getRawValue()).isEqualTo(0.0);
    }

    @Test
    void refusalAccuracyShouldDetectRefusal() {
        RagEvaluationCaseEntity evalCase = new RagEvaluationCaseEntity();
        evalCase.setQueryType(RagEvaluationQueryType.NO_ANSWER);

        List<HealthMetricValue> metrics = calculator.calculate(
                evalCase,
                "根据上下文不足，无法确定。",
                List.of(),
                List.of()
        );

        assertThat(metric(metrics, "REFUSAL_ACCURACY").getRawValue()).isEqualTo(1.0);
    }

    @Test
    void faithfulnessShouldBeHeuristic() {
        RagEvaluationCaseEntity evalCase = new RagEvaluationCaseEntity();
        List<HealthMetricValue> metrics = calculator.calculate(
                evalCase,
                "确定答案",
                List.of("1"),
                List.of(chunk(1L))
        );

        HealthMetricValue faithfulness = metric(metrics, "FAITHFULNESS");
        assertThat(faithfulness.isHeuristic()).isTrue();
        assertThat(faithfulness.isAvailable()).isTrue();
    }

    @Test
    void missingAnswerPointsShouldMarkCoverageUnavailable() {
        RagEvaluationCaseEntity evalCase = new RagEvaluationCaseEntity();
        List<HealthMetricValue> metrics = calculator.calculate(evalCase, "answer", List.of(), List.of());
        assertThat(metric(metrics, "ANSWER_COVERAGE").isAvailable()).isFalse();
    }

    private EvaluationRetrievedChunk chunk(Long id) {
        return new EvaluationRetrievedChunk(id, 1L, null, null, null, null, null, "ctx", 0.9, 1,
                RagEvaluationRetrievalSource.VECTOR, null, false, false, false, false);
    }

    private HealthMetricValue metric(List<HealthMetricValue> metrics, String code) {
        return metrics.stream().filter(m -> code.equals(m.getCode())).findFirst().orElseThrow();
    }
}
