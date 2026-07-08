package com.tuoman.ai_task_orchestrator.rag.quality;

import com.tuoman.ai_task_orchestrator.dto.RagGenerationMetadataResponse;
import com.tuoman.ai_task_orchestrator.dto.RagQualityDiagnosisResponse;
import com.tuoman.ai_task_orchestrator.dto.RagRetrievalMetadataResponse;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class RagQualityDiagnosisServiceTest {

    private RagQualityDiagnosisService diagnosisService;

    @BeforeEach
    void setUp() {
        diagnosisService = new RagQualityDiagnosisService();
    }

    @Test
    void shouldDiagnoseNoContext() {
        RagQualityScoreContext context = baseContext()
                .answer("根据当前检索到的文档内容，无法确定。")
                .citations(List.of())
                .retrieval(retrieval(0))
                .generation(generation(true, 100L))
                .build();
        RagQualityScoreResult score = score(15, 10, 35, 10);

        RagQualityDiagnosisResponse diagnosis = diagnosisService.diagnose(context, score);

        assertThat(diagnosis.getIssues()).extracting("code").contains("NO_CONTEXT");
        assertThat(diagnosis.getSummary()).contains("质量");
        assertThat(diagnosis.getSuggestions()).isNotEmpty();
        assertThat(diagnosis.getSuggestions().getFirst().getTitle()).isNotBlank();
    }

    @Test
    void shouldDiagnoseLowRetrievalRecall() {
        RagQualityScoreContext context = baseContext()
                .citations(List.of())
                .retrieval(retrieval(1))
                .generation(generation(true, 200L))
                .build();
        RagQualityScoreResult score = score(45, 55, 40, 30);

        RagQualityDiagnosisResponse diagnosis = diagnosisService.diagnose(context, score);

        assertThat(diagnosis.getIssues()).extracting("code").contains("LOW_RETRIEVAL_RECALL");
    }

    @Test
    void shouldDiagnoseLowContextPrecision() {
        RagQualityScoreContext context = baseContext()
                .retrieval(retrieval(2))
                .generation(generation(false, 300L))
                .build();
        RagQualityScoreResult score = score(70, 45, 70, 70);

        RagQualityDiagnosisResponse diagnosis = diagnosisService.diagnose(context, score);

        assertThat(diagnosis.getIssues()).extracting("code").contains("LOW_CONTEXT_PRECISION");
    }

    @Test
    void shouldDiagnoseWeakCitation() {
        RagQualityScoreContext context = baseContext()
                .retrieval(retrieval(1))
                .generation(generation(false, 400L))
                .build();
        RagQualityScoreResult score = score(70, 70, 70, 40);

        RagQualityDiagnosisResponse diagnosis = diagnosisService.diagnose(context, score);

        assertThat(diagnosis.getIssues()).extracting("code").contains("WEAK_CITATION");
    }

    @Test
    void shouldDiagnoseAnswerNotGroundedRisk() {
        RagQualityScoreContext context = baseContext()
                .answer("这是一个非常确定的答案，但没有引用。")
                .citations(List.of())
                .retrieval(retrieval(0))
                .generation(generation(false, 500L))
                .build();
        RagQualityScoreResult score = score(20, 15, 60, 15);

        RagQualityDiagnosisResponse diagnosis = diagnosisService.diagnose(context, score);

        assertThat(diagnosis.getIssues()).extracting("code").contains("ANSWER_NOT_GROUNDED_RISK");
    }

    @Test
    void shouldDiagnoseSlowResponse() {
        RagQualityScoreContext context = baseContext()
                .retrieval(retrieval(1))
                .generation(generation(false, 20_000L))
                .build();
        RagQualityScoreResult score = score(80, 80, 80, 80);

        RagQualityDiagnosisResponse diagnosis = diagnosisService.diagnose(context, score);

        assertThat(diagnosis.getIssues()).extracting("code").contains("SLOW_RESPONSE");
        assertThat(diagnosis.getSuggestions()).extracting("code").contains("SLOW_RESPONSE_TOPK");
    }

    @Test
    void summaryAndSuggestionsShouldBeChineseFriendly() {
        RagQualityScoreContext context = baseContext()
                .citations(List.of())
                .retrieval(retrieval(0))
                .generation(generation(true, null))
                .build();
        RagQualityScoreResult score = score(20, 10, 35, 10);

        RagQualityDiagnosisResponse diagnosis = diagnosisService.diagnose(context, score);

        assertThat(diagnosis.getSummary()).contains("本次回答质量");
        assertThat(diagnosis.getSuggestions())
                .allMatch(suggestion -> suggestion.getDescription().chars().anyMatch(ch -> ch > 127));
    }

    private RagQualityScoreContext.RagQualityScoreContextBuilder baseContext() {
        return RagQualityScoreContext.builder()
                .query("系统 Java Python 分工")
                .answer("Java 负责编排。")
                .mode(RagQualityMode.BALANCED);
    }

    private RagQualityScoreResult score(int retrieval, int context, int answer, int citation) {
        return RagQualityScoreResult.builder()
                .overallScore(60)
                .overallLevel(RagQualityLevel.FAIR)
                .retrievalScore(retrieval)
                .contextScore(context)
                .answerScore(answer)
                .citationScore(citation)
                .mode(RagQualityMode.BALANCED)
                .weights(RagQualityWeights.forMode(RagQualityMode.BALANCED))
                .build();
    }

    private RagRetrievalMetadataResponse retrieval(int finalContextCount) {
        return new RagRetrievalMetadataResponse(
                5,
                finalContextCount,
                "mock",
                "mock-embedding-v1",
                128,
                "ExactCosineVectorStore",
                null,
                null,
                null,
                5,
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

    private RagGenerationMetadataResponse generation(boolean skipped, Long latencyMs) {
        return new RagGenerationMetadataResponse(
                skipped ? null : "mock",
                skipped ? null : "mock-llm",
                skipped ? null : "mock",
                skipped ? null : "mock-llm",
                skipped,
                skipped ? "NO_RETRIEVED_CONTEXT" : null,
                latencyMs,
                null,
                null
        );
    }
}
