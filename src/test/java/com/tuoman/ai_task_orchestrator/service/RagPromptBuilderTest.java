package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.DocumentSearchResultResponse;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class RagPromptBuilderTest {

    private final RagPromptBuilder ragPromptBuilder = new RagPromptBuilder();

    @Test
    void buildPromptShouldIncludeQueryAndChunkContext() {
        String prompt = ragPromptBuilder.buildPrompt(
                "Why use outbox?",
                List.of(chunk(1L, 10L, 0, "Reliability", "Outbox keeps DB and MQ dispatch reliable."))
        );

        assertThat(prompt).contains("Question:");
        assertThat(prompt).contains("Why use outbox?");
        assertThat(prompt).contains("[1]");
        assertThat(prompt).contains("documentId: 1");
        assertThat(prompt).contains("chunkId: 10");
        assertThat(prompt).contains("headingPath: Reliability");
        assertThat(prompt).contains("Outbox keeps DB and MQ dispatch reliable.");
    }

    @Test
    void buildPromptShouldNumberMultipleChunksFromOne() {
        String prompt = ragPromptBuilder.buildPrompt(
                "How does retry work?",
                List.of(
                        chunk(1L, 10L, 0, "Retry", "First chunk"),
                        chunk(1L, 11L, 1, "Retry", "Second chunk")
                )
        );

        assertThat(prompt).contains("[1]");
        assertThat(prompt).contains("[2]");
        assertThat(prompt.indexOf("[1]")).isLessThan(prompt.indexOf("[2]"));
    }

    @Test
    void buildPromptShouldHandleEmptyChunks() {
        String prompt = ragPromptBuilder.buildPrompt("Unknown question", List.of());

        assertThat(prompt).contains("Question:");
        assertThat(prompt).contains("Unknown question");
        assertThat(prompt).contains("无可用资料。");
        assertThat(prompt).contains("Answer:");
    }

    private DocumentSearchResultResponse chunk(
            Long documentId,
            Long chunkId,
            Integer chunkIndex,
            String headingPath,
            String content
    ) {
        return new DocumentSearchResultResponse(
                documentId,
                chunkId,
                chunkIndex,
                0.91,
                content,
                content.length(),
                headingPath,
                0,
                content.length(),
                "RECURSIVE_TEXT",
                "mock",
                "mock-embedding-v1",
                "COSINE"
        );
    }
}
