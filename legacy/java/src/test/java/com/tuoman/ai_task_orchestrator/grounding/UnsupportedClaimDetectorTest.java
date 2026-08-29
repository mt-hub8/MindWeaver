package com.tuoman.ai_task_orchestrator.grounding;

import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class UnsupportedClaimDetectorTest {

    private final UnsupportedClaimDetector detector = new UnsupportedClaimDetector();

    @Test
    void keyClaimWithoutCitationShouldBeDetected() {
        UnsupportedClaimReport report = detector.detect("系统默认支持 app.demo.enabled。", bundle("app.demo.enabled=true"));

        assertThat(report.getMissingCitationClaims()).isEqualTo(1);
        assertThat(report.getUnsupportedClaims()).isEqualTo(1);
    }

    @Test
    void missingSymbolAndVersionShouldBeDetected() {
        UnsupportedClaimReport report = detector.detect(
                "LocalPythonLlmProvider 在 V10.0 支持配置 [1]",
                bundle("V9.0 支持模型配置。")
        );

        assertThat(report.getUnsupportedClaims()).isEqualTo(1);
        assertThat(report.getClaimDetails().getFirst().getIssueType()).isNotEqualTo(ClaimIssueType.NONE);
    }

    @Test
    void noContextDeterministicAnswerShouldMarkHallucinationRisk() {
        UnsupportedClaimReport report = detector.detect(
                "系统一定支持 /api/demo。",
                GroundedContextBundle.builder().contextId("empty").chunks(List.of()).citations(List.of()).build()
        );

        assertThat(report.isHallucinationRisk()).isTrue();
    }

    private GroundedContextBundle bundle(String text) {
        return GroundedContextBundle.builder()
                .contextId("ctx")
                .chunks(List.of(GroundedContextChunk.builder().chunkId(1L).text(text).citationKey("[1]").build()))
                .citations(List.of(Citation.builder().citationKey("[1]").chunkId(1L).build()))
                .build();
    }
}
