package com.tuoman.ai_task_orchestrator.grounding;

import com.fasterxml.jackson.annotation.JsonInclude;
import lombok.Getter;

import java.util.List;

@Getter
@JsonInclude(JsonInclude.Include.NON_NULL)
public class GroundedAnswerContract {

    private final AnswerContractMode mode;

    private final List<String> rules;

    public GroundedAnswerContract(AnswerContractMode mode) {
        this.mode = mode == null ? AnswerContractMode.defaultMode() : mode;
        this.rules = List.of(
                "Only answer from the final context.",
                "Do not use facts outside the provided context.",
                "Every key conclusion needs a citation key such as [1].",
                "Refuse or state uncertainty when context is insufficient.",
                "Never cite chunks that are not in the final context.",
                "Do not invent document names, section names, file paths, config keys, API paths, or versions.",
                "Mark inference as inference.",
                "Call out conflicts instead of merging conflicting evidence."
        );
    }

    public boolean strictCitationRequired() {
        return mode == AnswerContractMode.STRICT;
    }

    public boolean inferenceAllowed() {
        return mode == AnswerContractMode.EXPLORATORY;
    }
}
