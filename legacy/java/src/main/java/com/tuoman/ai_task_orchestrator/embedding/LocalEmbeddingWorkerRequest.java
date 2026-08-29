package com.tuoman.ai_task_orchestrator.embedding;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.util.List;

@Getter
@AllArgsConstructor
public class LocalEmbeddingWorkerRequest {

    private String model;

    private List<String> input;
}
