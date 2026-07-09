package com.tuoman.ai_task_orchestrator.grounding;

import com.tuoman.ai_task_orchestrator.queryunderstanding.QueryType;
import com.tuoman.ai_task_orchestrator.queryunderstanding.QueryUnderstandingResult;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class RefusalPolicyServiceTest {

    private final RefusalPolicyService service = new RefusalPolicyService();

    @Test
    void noContextAndAmbiguousQueryShouldRefuse() {
        GroundedContextBundle empty = GroundedContextBundle.builder().chunks(List.of()).citations(List.of()).build();

        assertThat(service.decideBeforeGeneration(empty, null, null, AnswerContractMode.BALANCED).isShouldRefuse()).isTrue();
        assertThat(service.decideBeforeGeneration(empty, ambiguous(), null, AnswerContractMode.BALANCED).getReasonCode())
                .isEqualTo(RefusalReasonCode.QUERY_AMBIGUOUS);
    }

    @Test
    void strictModeWithoutEvidenceShouldRefuse() {
        GroundedContextBundle bundle = GroundedContextBundle.builder()
                .chunks(List.of(GroundedContextChunk.builder().chunkId(1L).text("x").citationKey("[1]").build()))
                .citations(List.of())
                .build();

        assertThat(service.decideBeforeGeneration(bundle, null, null, AnswerContractMode.STRICT).getReasonCode())
                .isEqualTo(RefusalReasonCode.STRICT_MODE_NO_EVIDENCE);
    }

    @Test
    void allUnsupportedCitationsShouldRefuseAfterVerification() {
        CitationVerificationResult verification = CitationVerificationResult.builder()
                .totalCitations(2)
                .verifiedCitations(0)
                .unsupportedCitations(2)
                .build();

        assertThat(service.decideAfterVerification(verification, AnswerContractMode.BALANCED).isShouldRefuse()).isTrue();
    }

    private QueryUnderstandingResult ambiguous() {
        return QueryUnderstandingResult.builder()
                .originalQuery("啥")
                .normalizedQuery("啥")
                .queryType(QueryType.AMBIGUOUS)
                .confidence(0.2)
                .warnings(List.of())
                .reasons(List.of())
                .build();
    }
}
