package com.tuoman.ai_task_orchestrator.llm;

import com.fasterxml.jackson.annotation.JsonProperty;
import lombok.Getter;
import lombok.Setter;

import java.util.List;

@Getter
@Setter
public class OpenAiChatRequest {

    private String model;

    private List<OpenAiChatMessage> messages;

    private Double temperature;

    @JsonProperty("max_tokens")
    private Integer maxTokens;
}
