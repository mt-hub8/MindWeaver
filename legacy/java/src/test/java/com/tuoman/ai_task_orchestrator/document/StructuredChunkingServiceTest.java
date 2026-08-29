package com.tuoman.ai_task_orchestrator.document;

import com.tuoman.ai_task_orchestrator.config.ChunkingProperties;
import com.tuoman.ai_task_orchestrator.embedding.ChunkHashService;
import com.tuoman.ai_task_orchestrator.enums.ChunkType;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class StructuredChunkingServiceTest {

    private StructuredChunkingService service;

    @BeforeEach
    void setUp() {
        ChunkingProperties properties = new ChunkingProperties();
        properties.setMaxChars(1200);
        properties.setOverlapChars(120);
        properties.setIncludeSectionPath(true);
        service = new StructuredChunkingService(new DocumentChunker(), properties, new ChunkHashService());
    }

    @Test
    void markdownHeadingsShouldProduceSectionPath() {
        String content = "# Root\n\nIntro paragraph.\n\n## Child\n\nDetail about API.";
        List<StructuredChunkResult> chunks = service.chunk(content);
        assertThat(chunks).isNotEmpty();
        assertThat(chunks.stream().anyMatch(c -> c.getSectionPath() != null && c.getSectionPath().contains("Root"))).isTrue();
    }

    @Test
    void codeBlockShouldBeDetected() {
        String content = "Text\n\n```java\npublic class ApiClient {}\n```\n";
        List<StructuredChunkResult> chunks = service.chunk(content);
        assertThat(chunks.stream().anyMatch(c -> c.getChunkType() == ChunkType.CODE_BLOCK)).isTrue();
    }

    @Test
    void previousChunkIndexShouldLink() {
        ChunkingProperties properties = new ChunkingProperties();
        properties.setMaxChars(80);
        properties.setOverlapChars(0);
        StructuredChunkingService smallChunkService = new StructuredChunkingService(
                new DocumentChunker(), properties, new ChunkHashService());
        String content = "Paragraph one with enough text to force split. "
                + "Paragraph two with more text. Paragraph three continues.";
        List<StructuredChunkResult> chunks = smallChunkService.chunk(content);
        assertThat(chunks.size()).isGreaterThan(1);
        assertThat(chunks.get(1).getPreviousChunkIndex()).isNotNull();
    }

    @Test
    void contentHashShouldBePresent() {
        List<StructuredChunkResult> chunks = service.chunk("stable content");
        assertThat(chunks.get(0).getContentHash()).isNotBlank();
    }
}
