package com.tuoman.ai_task_orchestrator.prompt;

import org.junit.jupiter.api.Test;

import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class PromptTemplateRendererTest {

    private final PromptTemplateRenderer renderer = new PromptTemplateRenderer();

    @Test
    void render_shouldReplaceVariable() {
        String result = renderer.render(
                "用户输入：{{prompt}}",
                Map.of("prompt", "normal task")
        );

        assertEquals("用户输入：normal task", result);
    }

    @Test
    void render_shouldSupportSpacesAroundVariableName() {
        String result = renderer.render(
                "用户输入：{{ prompt }}",
                Map.of("prompt", "normal task")
        );

        assertEquals("用户输入：normal task", result);
    }

    @Test
    void render_shouldReplaceMultipleVariables() {
        String result = renderer.render(
                "taskId={{taskId}}, model={{model}}, prompt={{prompt}}",
                Map.of(
                        "taskId", "1",
                        "model", "mock-llm",
                        "prompt", "normal task"
                )
        );

        assertEquals("taskId=1, model=mock-llm, prompt=normal task", result);
    }

    @Test
    void render_shouldThrowWhenVariableIsMissing() {
        IllegalArgumentException exception = assertThrows(
                IllegalArgumentException.class,
                () -> renderer.render("用户输入：{{prompt}}", Map.of())
        );

        assertTrue(exception.getMessage().contains("prompt"));
    }

    @Test
    void render_shouldThrowWhenTemplateContentIsBlank() {
        IllegalArgumentException exception = assertThrows(
                IllegalArgumentException.class,
                () -> renderer.render("", Map.of("prompt", "normal task"))
        );

        assertEquals("Template content must not be blank", exception.getMessage());
    }
}
