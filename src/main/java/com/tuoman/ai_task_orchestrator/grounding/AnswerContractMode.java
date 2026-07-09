package com.tuoman.ai_task_orchestrator.grounding;

public enum AnswerContractMode {
    STRICT,
    BALANCED,
    EXPLORATORY;

    public static AnswerContractMode defaultMode() {
        return BALANCED;
    }
}
