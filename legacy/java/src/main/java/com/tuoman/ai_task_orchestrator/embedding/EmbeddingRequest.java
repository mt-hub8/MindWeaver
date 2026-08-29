package com.tuoman.ai_task_orchestrator.embedding;

import lombok.Getter;
import lombok.Setter;

@Getter
@Setter
public class EmbeddingRequest {

    private String text;

    private String model;
}
