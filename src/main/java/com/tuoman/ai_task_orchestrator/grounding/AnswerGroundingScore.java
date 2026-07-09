package com.tuoman.ai_task_orchestrator.grounding;

import com.fasterxml.jackson.annotation.JsonInclude;
import lombok.Builder;
import lombok.Getter;

@Getter
@Builder
@JsonInclude(JsonInclude.Include.NON_NULL)
public class AnswerGroundingScore {

    private final double citationCoverage;
    private final double citationAccuracy;
    private final double unsupportedClaimRate;
    private final double refusalCorrectness;
    private final double contextUsage;
    private final int groundingScore;
    private final String level;
    private final boolean heuristic;
}
