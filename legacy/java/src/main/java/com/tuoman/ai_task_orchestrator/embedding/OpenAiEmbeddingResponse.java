package com.tuoman.ai_task_orchestrator.embedding;

import com.fasterxml.jackson.annotation.JsonProperty;
import lombok.Getter;
import lombok.Setter;

import java.util.List;

@Getter
@Setter
public class OpenAiEmbeddingResponse {

    private List<EmbeddingData> data;

    private String model;

    private Usage usage;

    @Getter
    @Setter
    public static class EmbeddingData {

        private Integer index;

        private List<Double> embedding;
    }

    @Getter
    @Setter
    public static class Usage {

        @JsonProperty("prompt_tokens")
        private Integer promptTokens;

        @JsonProperty("total_tokens")
        private Integer totalTokens;
    }
}
