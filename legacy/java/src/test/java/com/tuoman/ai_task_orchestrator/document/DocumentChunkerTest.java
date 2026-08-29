package com.tuoman.ai_task_orchestrator.document;

import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class DocumentChunkerTest {

    private final DocumentChunker documentChunker = new DocumentChunker();

    @Test
    void shouldCreateOneChunkForShortText() {
        List<DocumentChunkResult> chunks = documentChunker.chunk("short text");

        assertThat(chunks).hasSize(1);
        assertThat(chunks.get(0).getContent()).isEqualTo("short text");
        assertThat(chunks.get(0).getChunkStrategy()).isEqualTo(DocumentChunker.CHUNK_STRATEGY);
    }

    @Test
    void shouldCreateMultipleChunksForLongText() {
        String content = ("第一段内容很长，用来测试递归切分。\n\n").repeat(80);

        List<DocumentChunkResult> chunks = documentChunker.chunk(content);

        assertThat(chunks).hasSizeGreaterThan(1);
    }

    @Test
    void shouldCreateChunkIndexFromZero() {
        String content = ("chunk index test.\n\n").repeat(120);

        List<DocumentChunkResult> chunks = documentChunker.chunk(content);

        for (int i = 0; i < chunks.size(); i++) {
            assertThat(chunks.get(i).getChunkIndex()).isEqualTo(i);
        }
    }

    @Test
    void shouldNotClearlyExceedChunkSizeWithOverlap() {
        String content = ("长度测试内容。\n\n").repeat(200);

        List<DocumentChunkResult> chunks = documentChunker.chunk(content);

        assertThat(chunks).allSatisfy(chunk ->
                assertThat(chunk.getContentLength()).isLessThanOrEqualTo(1150)
        );
    }

    @Test
    void shouldApplyOverlapBetweenAdjacentChunks() {
        String content = "a".repeat(1000) + "b".repeat(1000);

        List<DocumentChunkResult> chunks = documentChunker.chunk(content);

        assertThat(chunks).hasSize(2);
        assertThat(chunks.get(1).getStartOffset()).isEqualTo(850);
        assertThat(chunks.get(1).getContent()).startsWith("a".repeat(150));
    }

    @Test
    void shouldWriteHeadingPathForMarkdownContent() {
        String content = "# 一级标题\n\n## 二级标题\n\n这里是标题下的内容。";

        List<DocumentChunkResult> chunks = documentChunker.chunk(content);

        assertThat(chunks)
                .extracting(DocumentChunkResult::getHeadingPath)
                .contains("一级标题 > 二级标题");
    }

    @Test
    void shouldReturnEmptyChunksForBlankText() {
        assertThat(documentChunker.chunk("   ")).isEmpty();
    }

    @Test
    void shouldSplitChinesePunctuationWithoutBlankChunks() {
        String content = ("这是第一句。这是第二句？这是第三句！").repeat(80);

        List<DocumentChunkResult> chunks = documentChunker.chunk(content);

        assertThat(chunks).hasSizeGreaterThan(1);
        assertThat(chunks).allSatisfy(chunk -> {
            assertThat(chunk.getContent()).isNotBlank();
            assertThat(chunk.getChunkStrategy()).isNotBlank();
        });
    }

    @Test
    void shouldSplitEnglishPunctuationWithoutBlankChunks() {
        String content = ("This is sentence one. Is this sentence two? This is sentence three! ").repeat(80);

        List<DocumentChunkResult> chunks = documentChunker.chunk(content);

        assertThat(chunks).hasSizeGreaterThan(1);
        assertThat(chunks).allSatisfy(chunk -> assertThat(chunk.getContent()).isNotBlank());
    }

    @Test
    void shouldCreateValidOffsets() {
        String content = ("offset test paragraph.\n\n").repeat(100);

        List<DocumentChunkResult> chunks = documentChunker.chunk(content);

        assertThat(chunks).allSatisfy(chunk -> {
            assertThat(chunk.getStartOffset()).isGreaterThanOrEqualTo(0);
            assertThat(chunk.getEndOffset()).isGreaterThan(chunk.getStartOffset());
            assertThat(chunk.getEndOffset()).isLessThanOrEqualTo(content.length());
        });
    }
}
