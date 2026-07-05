package com.tuoman.ai_task_orchestrator.rag.quality;

import lombok.Builder;
import lombok.Getter;

import java.util.Map;

@Getter
@Builder
public class RagQualityScoreResult {

    private final int overallScore;

    private final RagQualityLevel overallLevel;

    private final int retrievalScore;

    private final int contextScore;

    private final int answerScore;

    private final int citationScore;

    private final RagQualityMode mode;

    private final RagQualityWeights weights;

    private final Map<String, Object> metricDetails;

    private final String scoringNote;
}
