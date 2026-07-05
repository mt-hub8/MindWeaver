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
                DocumentLifecycleStatus.TRASHED,
                DocumentStatus.READY,
                false
        )).contains("垃圾箱");

        assertThat(DocumentLifecycleDisplayTexts.deleteSuccessMessage())
                .contains("垃圾箱");
    }
}
