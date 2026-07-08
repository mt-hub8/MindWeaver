package com.tuoman.ai_task_orchestrator.documentation;

import org.junit.jupiter.api.Test;

import java.nio.file.Files;
import java.nio.file.Path;

import static org.assertj.core.api.Assertions.assertThat;

class QueryUnderstandingUiDocumentationTest {

    @Test
    void askPageShouldContainQueryUnderstandingCopy() throws Exception {
        String html = Files.readString(Path.of("src/main/resources/static/ask.html"));

        assertThat(html).contains("查询理解");
        assertThat(html).contains("检索路由");
        assertThat(html).contains("改写后的查询");
    }

    @Test
    void scriptsShouldContainClarificationCopyAndSettingsEntry() throws Exception {
        String askHtml = Files.readString(Path.of("src/main/resources/static/ask.html"));
        String settingsHtml = Files.readString(Path.of("src/main/resources/static/retrieval-settings.html"));

        assertThat(askHtml).contains("当前问题比较模糊");
        assertThat(settingsHtml).contains("查询理解设置");
    }
}
