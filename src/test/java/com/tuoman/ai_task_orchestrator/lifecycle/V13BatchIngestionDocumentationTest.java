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
class V13BatchIngestionDocumentationTest {

    private static final Path MANUAL = Path.of("docs/manual/batch-ingestion-and-notification.md");
    private static final Path README = Path.of("README.md");

    @Autowired
    private MockMvc mockMvc;

    @Test
    void batchIngestionPageShouldExistWithRequiredCopy() throws Exception {
        String html = fetchUtf8("/batch-ingestion.html");
        assertThat(html).contains("批量导入");
        assertThat(html).contains("重试失败文件");
        assertThat(html).contains("跳过重复");
        assertThat(html).contains("取消剩余任务");
        assertThat(html).contains("取消不会删除已成功导入的文档");
    }

    @Test
    void notificationsPageShouldExistWithRequiredCopy() throws Exception {
        String html = fetchUtf8("/notifications.html");
        assertThat(html).contains("通知中心");
        assertThat(html).contains("标记已读");
    }

    @Test
    void indexAndDocumentsShouldLinkBatchIngestion() throws Exception {
        assertThat(fetchUtf8("/index.html")).contains("批量导入资料");
        assertThat(fetchUtf8("/documents.html")).contains("批量导入");
    }

    @Test
    void manualAndReadmeShouldDocumentV13() throws Exception {
        assertThat(MANUAL).exists();
        String manual = Files.readString(MANUAL, StandardCharsets.UTF_8);
        assertThat(manual).contains("V13.0");
        assertThat(manual).contains("文件级去重");
        assertThat(manual).contains("文本级去重");

        String readme = Files.readString(README, StandardCharsets.UTF_8);
        assertThat(readme).contains("批量导入");
        assertThat(readme).contains("通知中心");
        assertThat(readme).contains("文件级去重");
        assertThat(readme).contains("文本级去重");
        assertThat(readme).contains("V13.0");
    }

    private String fetchUtf8(String path) throws Exception {
        MvcResult result = mockMvc.perform(get(path))
                .andExpect(status().isOk())
                .andReturn();
        return result.getResponse().getContentAsString(StandardCharsets.UTF_8);
    }
}
