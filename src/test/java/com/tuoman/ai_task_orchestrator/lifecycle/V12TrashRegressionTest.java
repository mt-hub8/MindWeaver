package com.tuoman.ai_task_orchestrator.lifecycle;

import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.AutoConfigureMockMvc;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.test.web.servlet.MockMvc;
import org.springframework.test.web.servlet.MvcResult;

import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;

import static org.assertj.core.api.Assertions.assertThat;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@SpringBootTest
@AutoConfigureMockMvc
class V12TrashRegressionTest {

    @Autowired
    private MockMvc mockMvc;

    @Test
    void trashPageShouldExposeRequiredCopy() throws Exception {
        String html = fetchUtf8("/trash.html");
        assertThat(html).contains("垃圾箱");
        assertThat(html).contains("恢复");
        assertThat(html).contains("永久删除");
        assertThat(html).contains("7 天后将自动永久删除");
        assertThat(html).contains("永久删除后不可恢复");
        assertThat(html).contains("查看技术详情");
    }

    @Test
    void settingsPageShouldExposeStorageAndCacheManagement() throws Exception {
        String html = fetchUtf8("/settings.html");
        assertThat(html).contains("存储空间");
        assertThat(html).contains("缓存管理");
        assertThat(html).contains("清理缓存");
        assertThat(html).contains("清理缓存不会删除原始文档");

        String js = fetchUtf8("/settings.js");
        assertThat(js).contains("/storage/summary");
        assertThat(js).contains("/storage/cache/clear-all");
    }

    @Test
    void manualAndReadmeShouldDocumentTrashLifecycle() throws Exception {
        Path manual = Path.of("docs/manual/trash-and-local-storage-management.md");
        assertThat(Files.exists(manual)).isTrue();
        String manualContent = Files.readString(manual, StandardCharsets.UTF_8);
        assertThat(manualContent).contains("V12.0");
        assertThat(manualContent).contains("TRASHED");

        String readme = Files.readString(Path.of("README.md"), StandardCharsets.UTF_8);
        assertThat(readme).contains("垃圾箱");
        assertThat(readme).contains("ACTIVE");
        assertThat(readme).contains("TRASHED");
        assertThat(readme).contains("PURGED");
        assertThat(readme).contains("7 天");
        assertThat(readme).contains("缓存管理");
        assertThat(readme).contains("Batch Ingestion");
    }

    private String fetchUtf8(String path) throws Exception {
        MvcResult result = mockMvc.perform(get(path))
                .andExpect(status().isOk())
                .andReturn();
        return result.getResponse().getContentAsString(StandardCharsets.UTF_8);
    }
}
