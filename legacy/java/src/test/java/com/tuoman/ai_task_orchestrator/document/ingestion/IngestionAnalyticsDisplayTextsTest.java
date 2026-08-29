package com.tuoman.ai_task_orchestrator.document.ingestion;

import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

class IngestionAnalyticsDisplayTextsTest {

    @Test
    void shouldMapStageAndFailureMessagesToChinese() {
        assertThat(IngestionAnalyticsDisplayTexts.stageDisplayName("CHUNKING")).isEqualTo("文档切分");
        assertThat(IngestionAnalyticsDisplayTexts.stageDisplayName("EMBEDDING")).isEqualTo("生成文档向量");
        assertThat(IngestionAnalyticsDisplayTexts.stageDisplayName("VECTOR_WRITING")).isEqualTo("写入知识库索引");
        assertThat(IngestionAnalyticsDisplayTexts.failureReasonDisplayMessage("UNKNOWN_ERROR", null))
                .isEqualTo("未知错误");
        assertThat(IngestionAnalyticsDisplayTexts.failureReasonDisplayMessage("PDF_NO_EXTRACTABLE_TEXT", null))
                .isEqualTo("PDF 没有可提取文字");
    }
}
