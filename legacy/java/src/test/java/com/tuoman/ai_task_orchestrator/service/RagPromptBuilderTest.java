package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.RagCitationResponse;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class RagPromptBuilderTest {

    private final RagPromptBuilder ragPromptBuilder = new RagPromptBuilder();

    @Test
    void buildPromptShouldIncludeQueryAndNumberedContext() {
        String prompt = ragPromptBuilder.buildPrompt(
                "Why use outbox?",
                List.of(citation(1, 1L, 10L, "Outbox keeps DB and MQ dispatch reliable."))
        );

        assertThat(prompt).contains("用户问题：");
        assertThat(prompt).contains("Why use outbox?");
        assertThat(prompt).contains("[1]");
        assertThat(prompt).contains("documentId: 1");
        assertThat(prompt).contains("chunkId: 10");
        assertThat(prompt).contains("Outbox keeps DB and MQ dispatch reliable.");
        assertThat(prompt).contains("请给出简洁、准确、可追溯的回答。");
    }

    @Test
    void buildPromptShouldNumberMultipleCitationsFromOne() {
        String prompt = ragPromptBuilder.buildPrompt(
                "How does retry work?",
                List.of(
                        citation(1, 1L, 10L, "First chunk"),
                        citation(2, 1L, 11L, "Second chunk")
                )
        );

        assertThat(prompt).contains("[1]");
        assertThat(prompt).contains("[2]");
        assertThat(prompt.indexOf("[1]")).isLessThan(prompt.indexOf("[2]"));
    }

    @Test
    void buildPromptShouldRejectEmptyCitations() {
        assertThatThrownBy(() -> ragPromptBuilder.buildPrompt("Unknown question", List.of()))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("empty");
    }

    @Test
    void buildPromptShouldTruncateLongContent() {
        String longContent = "x".repeat(3000);
        String prompt = ragPromptBuilder.buildPrompt(
                "query",
                List.of(citation(1, 1L, 10L, longContent))
        );

        assertThat(prompt).doesNotContain(longContent);
        assertThat(prompt).contains("x".repeat(2000));
    }

    private RagCitationResponse citation(int sourceIndex, Long documentId, Long chunkId, String contentSnippet) {
        return new RagCitationResponse(sourceIndex, documentId, chunkId, 0.9, contentSnippet);
    }
}
