package com.tuoman.ai_task_orchestrator.rag.quality;

import lombok.Getter;

@Getter
public class RagQualityWeights {

    private final double retrieval;

    private final double context;

    private final double answer;

    private final double citation;

    private RagQualityWeights(double retrieval, double context, double answer, double citation) {
        this.retrieval = retrieval;
        this.context = context;
        this.answer = answer;
        this.citation = citation;
    }

    public static RagQualityWeights forMode(RagQualityMode mode) {
        return switch (mode) {
            case BALANCED -> new RagQualityWeights(0.30, 0.25, 0.25, 0.20);
            case PRECISE -> new RagQualityWeights(0.20, 0.30, 0.30, 0.20);
            case COMPREHENSIVE -> new RagQualityWeights(0.40, 0.20, 0.20, 0.20);
        };
    }
}
