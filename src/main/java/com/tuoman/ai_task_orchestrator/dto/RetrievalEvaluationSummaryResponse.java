package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class RetrievalEvaluationSummaryResponse {

    private Integer k;

    private Double recallAtK;

    private Double precisionAtK;

    private Double hitRateAtK;

    private Double mrr;

    private Double ndcgAtK;

    private Double contextPrecisionAtK;
}
