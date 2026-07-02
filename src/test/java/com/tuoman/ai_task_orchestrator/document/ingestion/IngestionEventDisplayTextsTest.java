package com.tuoman.ai_task_orchestrator.document.ingestion;

import com.tuoman.ai_task_orchestrator.enums.IngestionEventType;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

class IngestionEventDisplayTextsTest {

    @Test
    void shouldMapReindexEventTypesToChinese() {
        assertThat(IngestionEventDisplayTexts.displayEventType(IngestionEventType.DOCUMENT_REINDEX_REQUESTED))
                .isEqualTo("用户请求重新建立索引");
        assertThat(IngestionEventDisplayTexts.displayEventType(IngestionEventType.DOCUMENT_REINDEX_COMPLETED))
                .isEqualTo("重新索引完成");
        assertThat(IngestionEventDisplayTexts.displayEventType(IngestionEventType.DOCUMENT_REINDEX_FAILED))
                .isEqualTo("重新索引失败");
    }
}
