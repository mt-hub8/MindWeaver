package com.tuoman.ai_task_orchestrator.grounding;

import com.fasterxml.jackson.annotation.JsonInclude;
import lombok.Builder;
import lombok.Getter;

import java.util.List;

@Getter
@Builder
@JsonInclude(JsonInclude.Include.NON_NULL)
public class ClaimDetail {

    private final String claimText;
    private final int sentenceIndex;
    private final List<String> citationKeys;
    private final ClaimIssueType issueType;
    private final String severity;
    private final String reason;
}
