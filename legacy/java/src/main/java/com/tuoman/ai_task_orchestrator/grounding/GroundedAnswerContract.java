package com.tuoman.ai_task_orchestrator.grounding;

import com.fasterxml.jackson.annotation.JsonInclude;
import lombok.Getter;

import java.util.List;

@Getter
@JsonInclude(JsonInclude.Include.NON_NULL)
/**
 * V18 Grounded Answer Contract。
 *
 * Contract 描述回答生成必须遵守的边界：只用 final context、关键结论带 citation、
 * context 不足时拒答、不能引用不在 final context 中的 chunk。
 *
 * STRICT 要求更强 citation，BALANCED 是默认可信问答模式，EXPLORATORY 允许轻度推断但必须标注。
 */
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
