package com.tuoman.ai_task_orchestrator.rag.quality;

public enum RagQualityMode {
    BALANCED,
    PRECISE,
    COMPREHENSIVE;

    public static RagQualityMode fromRequest(String value) {
        if (value == null || value.isBlank()) {
            return BALANCED;
        }
        try {
            return RagQualityMode.valueOf(value.trim().toUpperCase());
        } catch (IllegalArgumentException exception) {
            return BALANCED;
        }
    }

    public String displayName() {
        return switch (this) {
            case BALANCED -> "平衡模式";
            case PRECISE -> "精准模式";
            case COMPREHENSIVE -> "全面模式";
        };
    }
}
