package com.tuoman.ai_task_orchestrator.rag.quality;

public enum RagQualityLevel {
    EXCELLENT,
    GOOD,
    FAIR,
    POOR,
    UNKNOWN;

    public String displayName() {
        return switch (this) {
            case EXCELLENT -> "优秀";
            case GOOD -> "良好";
            case FAIR -> "一般";
            case POOR -> "较差";
            case UNKNOWN -> "暂无评分";
        };
    }

    public static RagQualityLevel fromScore(int score) {
        if (score < 0) {
            return UNKNOWN;
        }
        if (score >= 85) {
            return EXCELLENT;
        }
        if (score >= 70) {
            return GOOD;
        }
        if (score >= 50) {
            return FAIR;
        }
        return POOR;
    }
}
