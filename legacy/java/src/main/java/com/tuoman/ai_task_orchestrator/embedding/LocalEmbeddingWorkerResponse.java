package com.tuoman.ai_task_orchestrator.embedding;

import lombok.Getter;
import lombok.Setter;

import java.util.List;

@Getter
@Setter
public class LocalEmbeddingWorkerResponse {

    private String provider;

    private String model;

    private Integer dimension;

    private List<EmbeddingData> data;

    @Getter
    @Setter
    public static class EmbeddingData {

        private Integer index;

        private List<Double> embedding;
    }
}
