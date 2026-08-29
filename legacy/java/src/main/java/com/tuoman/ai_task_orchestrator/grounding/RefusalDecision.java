package com.tuoman.ai_task_orchestrator.grounding;

import com.fasterxml.jackson.annotation.JsonInclude;
import lombok.Builder;
import lombok.Getter;

import java.util.List;

@Getter
@Builder
@JsonInclude(JsonInclude.Include.NON_NULL)
public class RefusalDecision {

    private final boolean shouldRefuse;
    private final RefusalReasonCode reasonCode;
    private final String reasonText;
    private final String suggestedAnswer;
    private final List<String> suggestedNextActions;

    public static RefusalDecision allow() {
        return RefusalDecision.builder()
                .shouldRefuse(false)
                .reasonCode(RefusalReasonCode.NONE)
                .suggestedNextActions(List.of())
                .build();
    }
}
