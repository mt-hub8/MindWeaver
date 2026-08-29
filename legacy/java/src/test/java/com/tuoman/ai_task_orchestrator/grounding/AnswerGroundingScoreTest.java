package com.tuoman.ai_task_orchestrator.grounding;

import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class AnswerGroundingScoreTest {

    private final AnswerGroundingScoreCalculator calculator = new AnswerGroundingScoreCalculator();

    @Test
    void shouldCalculateScoreAndLevel() {
        AnswerGroundingScore score = calculator.calculate(
                GroundedContextBundle.builder()
                        .chunks(List.of(GroundedContextChunk.builder().chunkId(1L).text("x").citationKey("[1]").build()))
                        .citations(List.of(Citation.builder().citationKey("[1]").build()))
                        .build(),
                CitationVerificationResult.builder().citationAccuracy(1.0).totalCitations(1).verifiedCitations(1).build(),
                UnsupportedClaimReport.builder().totalClaims(2).supportedClaims(1).unsupportedClaims(1).missingCitationClaims(0).claimDetails(List.of()).build(),
                RefusalDecision.allow()
        );

        assertThat(score.getGroundingScore()).isBetween(0, 100);
        assertThat(score.getCitationCoverage()).isEqualTo(1.0);
        assertThat(score.getCitationAccuracy()).isEqualTo(1.0);
        assertThat(score.getUnsupportedClaimRate()).isEqualTo(0.5);
        assertThat(score.getLevel()).isIn("可信", "基本可信", "需要复核", "不可信");
    }
}
