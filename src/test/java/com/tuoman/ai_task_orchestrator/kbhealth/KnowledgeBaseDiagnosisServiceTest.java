package com.tuoman.ai_task_orchestrator.kbhealth;

import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class KnowledgeBaseDiagnosisServiceTest {

    private final KnowledgeBaseDiagnosisService service = new KnowledgeBaseDiagnosisService();

    @Test
    void shouldDiagnoseLowRecallAndSuggestChineseActions() {
        var result = service.diagnose(List.of(HealthMetricValue.of("RECALL_AT_K", "Recall", 0.4, false)));
        assertThat(result.getIssues()).extracting(KnowledgeBaseDiagnosisService.DiagnosisIssue::getCode)
                .contains("LOW_RECALL");
        assertThat(result.getSuggestions()).isNotEmpty();
        assertThat(result.getSuggestions().get(0).getTitle()).contains("chunk");
    }

    @Test
    void shouldDiagnoseAllMajorRules() {
        var metrics = List.of(
                HealthMetricValue.of("MRR_AT_K", "MRR", 0.4, false),
                HealthMetricValue.of("NDCG_AT_K", "NDCG", 0.5, false),
                HealthMetricValue.of("CONTEXT_PRECISION_AT_K", "Precision", 0.4, false),
                HealthMetricValue.ofLeak("CROSS_COLLECTION_LEAK_RATE", "Cross", 0.2),
                HealthMetricValue.ofLeak("WRONG_VERSION_LEAK_RATE", "Version", 0.2),
                HealthMetricValue.of("CITATION_ACCURACY", "Citation", 0.5, false),
                HealthMetricValue.of("FAITHFULNESS", "Faith", 0.5, true),
                HealthMetricValue.of("REFUSAL_ACCURACY", "Refusal", 0.5, false)
        );
        var result = service.diagnose(metrics);
        assertThat(result.getIssues()).extracting(KnowledgeBaseDiagnosisService.DiagnosisIssue::getCode)
                .contains(
                        "LOW_MRR",
                        "LOW_NDCG",
                        "LOW_CONTEXT_PRECISION",
                        "CROSS_COLLECTION_LEAK",
                        "WRONG_VERSION_LEAK",
                        "LOW_CITATION_ACCURACY",
                        "LOW_FAITHFULNESS",
                        "LOW_REFUSAL_ACCURACY"
                );
    }

    @Test
    void metadataMissingShouldBeDiagnosed() {
        var result = service.diagnose(List.of(), 2);
        assertThat(result.getIssues()).extracting(KnowledgeBaseDiagnosisService.DiagnosisIssue::getCode)
                .contains("METADATA_MISSING");
        assertThat(result.getSuggestions().get(0).getTitle()).contains("元数据");
    }
}
