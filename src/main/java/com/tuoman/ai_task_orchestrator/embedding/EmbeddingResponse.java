package com.tuoman.ai_task_orchestrator.embedding;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.util.List;

@Getter
@AllArgsConstructor
public class EmbeddingResponse {

    private String provider;

    private String model;

    private Integer dimension;

    private String distanceMetric;

    private List<Double> vector;
}
