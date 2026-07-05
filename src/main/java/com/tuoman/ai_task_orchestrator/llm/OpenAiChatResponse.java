package com.tuoman.ai_task_orchestrator.llm;

import lombok.Getter;
import lombok.Setter;

import java.util.List;

@Getter
@Setter
public class OpenAiChatResponse {

    private List<Choice> choices;

    private Usage usage;

    @Getter
    @Setter
    public static class Choice {
        private OpenAiChatMessage message;
        private String finishReason;
    }

    @Getter
    @Setter
    public static class Usage {
        private Integer promptTokens;
        private Integer completionTokens;
    }
}
