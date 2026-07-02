package com.tuoman.ai_task_orchestrator.document.lifecycle;

import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

class DocumentLifecycleDisplayTextsTest {

    @Test
    void shouldProvideChineseLifecycleHints() {
        assertThat(DocumentLifecycleDisplayTexts.lifecycleHint(
                DocumentLifecycleStatus.ACTIVE,
                DocumentStatus.READY,
                true
        )).isEqualTo("当前文档可以用于知识库问答。");

        assertThat(DocumentLifecycleDisplayTexts.lifecycleHint(
                DocumentLifecycleStatus.DELETED,
                DocumentStatus.READY,
                false
        )).isEqualTo("该文档已删除，不会再用于知识库问答。");
    }

    @Test
    void shouldProvideDeleteSuccessMessage() {
        assertThat(DocumentLifecycleDisplayTexts.deleteSuccessMessage())
                .contains("删除成功")
                .contains("不会再用于知识库问答");
    }
}
