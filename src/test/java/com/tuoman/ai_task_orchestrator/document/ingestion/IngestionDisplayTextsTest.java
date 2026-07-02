package com.tuoman.ai_task_orchestrator.document.ingestion;

import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStep;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

class IngestionDisplayTextsTest {

    @Test
    void shouldMapStatusAndStepToChinese() {
        assertThat(IngestionDisplayTexts.displayStatus(IngestionTaskStatus.PENDING)).isEqualTo("待处理");
        assertThat(IngestionDisplayTexts.displayStatus(IngestionTaskStatus.PROCESSING)).isEqualTo("处理中");
        assertThat(IngestionDisplayTexts.displayStatus(IngestionTaskStatus.COMPLETED)).isEqualTo("已完成");
        assertThat(IngestionDisplayTexts.displayStatus(IngestionTaskStatus.FAILED)).isEqualTo("失败");

        assertThat(IngestionDisplayTexts.displayStep(IngestionTaskStep.EMBEDDING)).isEqualTo("正在生成文档向量");
        assertThat(IngestionDisplayTexts.displayStep(IngestionTaskStep.VECTOR_WRITING)).isEqualTo("正在写入知识库索引");
    }
}
