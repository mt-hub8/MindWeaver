package com.tuoman.ai_task_orchestrator.grounding;

import com.fasterxml.jackson.annotation.JsonInclude;
import lombok.Builder;
import lombok.Getter;

@Getter
@Builder
@JsonInclude(JsonInclude.Include.NON_NULL)
public class GroundedAnswerDiagnostics {

    private final GroundedAnswerContract contract;
    private final GroundedContextBundle contextBundle;
    private final CitationVerificationResult citationVerification;
    private final UnsupportedClaimReport unsupportedClaimReport;
    private final RefusalDecision refusalDecision;
    private final AnswerGroundingScore groundingScore;
}
