package com.tuoman.ai_task_orchestrator.llm;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class OpenAiChatMessage {

    private final String role;

    private final String content;
}
