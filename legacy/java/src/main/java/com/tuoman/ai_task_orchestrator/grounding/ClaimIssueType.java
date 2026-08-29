package com.tuoman.ai_task_orchestrator.grounding;

public enum ClaimIssueType {
    NONE,
    MISSING_CITATION,
    INVALID_CITATION,
    UNSUPPORTED_SYMBOL,
    UNSUPPORTED_VERSION,
    UNSUPPORTED_NUMBER,
    HALLUCINATION_RISK
}
