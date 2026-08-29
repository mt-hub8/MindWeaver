package com.tuoman.ai_task_orchestrator.grounding;

import com.fasterxml.jackson.annotation.JsonInclude;
import lombok.Builder;
import lombok.Getter;

import java.util.List;

@Getter
@Builder
@JsonInclude(JsonInclude.Include.NON_NULL)
public class UnsupportedClaimReport {

    private final int totalClaims;
    private final int supportedClaims;
    private final int unsupportedClaims;
    private final int missingCitationClaims;
    private final boolean hallucinationRisk;
    private final List<ClaimDetail> claimDetails;
}
