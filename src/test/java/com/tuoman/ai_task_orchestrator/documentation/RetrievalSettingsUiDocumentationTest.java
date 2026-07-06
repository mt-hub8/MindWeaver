package com.tuoman.ai_task_orchestrator.documentation;

import org.junit.jupiter.api.Test;

import java.nio.file.Files;
import java.nio.file.Path;

import static org.assertj.core.api.Assertions.assertThat;

class RetrievalSettingsUiDocumentationTest {

    @Test
    void retrievalSettingsPageShouldExistWithRequiredCopy() throws Exception {
        Path page = Path.of("src/main/resources/static/retrieval-settings.html");
        assertThat(page).exists();
        String content = Files.readString(page);
        assertThat(content).contains("检索设置");
        assertThat(content).contains("结构化切分");
        assertThat(content).contains("混合检索");
        assertThat(content).contains("RRF");
        assertThat(content).contains("重排序");
        assertThat(content).contains("上下文回填");
        assertThat(content).contains("重新索引");
    }

    @Test
    void askAndKnowledgeHealthShouldReferenceRetrievalStrategy() throws Exception {
        String ask = Files.readString(Path.of("src/main/resources/static/ask.html"));
        String health = Files.readString(Path.of("src/main/resources/static/knowledge-health.html"));
        assertThat(ask).contains("当前检索策略");
        assertThat(health).contains("根据诊断调整检索设置");
    }
}
