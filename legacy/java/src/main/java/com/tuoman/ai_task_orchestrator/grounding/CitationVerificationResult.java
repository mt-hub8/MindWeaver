package com.tuoman.ai_task_orchestrator.grounding;

import com.fasterxml.jackson.annotation.JsonInclude;
import lombok.Builder;
import lombok.Getter;

import java.util.List;

@Getter
@Builder
@JsonInclude(JsonInclude.Include.NON_NULL)
public class CitationVerificationResult {

    private final boolean valid;
    private final double citationAccuracy;
    private final int totalCitations;
    private final int verifiedCitations;
    private final int weakCitations;
    private final int unsupportedCitations;
    private final int missingCitationCount;
    private final int invalidCitationCount;
    private final boolean heuristic;
    private final List<String> warnings;
    private final List<Citation> citationDetails;
}
