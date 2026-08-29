package com.tuoman.ai_task_orchestrator.kbhealth;

public enum RagEvaluationQueryType {
    SINGLE_DOC_FACT,
    MULTI_DOC_COMPARE,
    VERSION_SPECIFIC,
    METADATA_FILTER,
    CODE_SYMBOL,
    CONFIG_KEY,
    NO_ANSWER,
    AMBIGUOUS,
    SUMMARY,
    CROSS_COLLECTION,
    WRONG_VERSION_TRAP
}
