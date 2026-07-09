package com.tuoman.ai_task_orchestrator.grounding;

import com.fasterxml.jackson.annotation.JsonInclude;
import lombok.Builder;
import lombok.Getter;

import java.util.List;

@Getter
@Builder
@JsonInclude(JsonInclude.Include.NON_NULL)
public class GroundedContextBundle {

    private final String contextId;
    private final String query;
    private final List<GroundedContextChunk> chunks;
    private final List<Citation> citations;
    private final int contextBudget;
    private final int usedChars;
    private final int usedTokensEstimate;
    private final boolean truncated;
    private final List<String> warnings;

    public boolean isEmpty() {
        return chunks == null || chunks.isEmpty();
    }
}
