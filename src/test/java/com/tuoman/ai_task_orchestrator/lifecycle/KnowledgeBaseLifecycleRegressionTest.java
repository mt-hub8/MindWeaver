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

/**
 * V4.0 阶段级回归：生命周期 UI 文案、manual 与跨 Batch 能力收敛验收。
 */
@SpringBootTest
@AutoConfigureMockMvc
class KnowledgeBaseLifecycleRegressionTest {

    @Autowired
    private MockMvc mockMvc;

    @Test
    void manualDocumentShouldExistWithLifecycleSections() throws Exception {
        Path manualPath = Path.of("docs/manual/knowledge-base-lifecycle-management.md");
        assertThat(Files.exists(manualPath)).isTrue();
        String content = Files.readString(manualPath, StandardCharsets.UTF_8);
        assertThat(content).contains("V4.0");
        assertThat(content).contains("软删除");
        assertThat(content).contains("重新建立索引");
        assertThat(content).contains("旧版本片段");
        assertThat(content).contains("物理删除");
    }

    @Test
    void documentsPageShouldExposeFullLifecycleManagementUi() throws Exception {
        String html = fetchUtf8("/documents.html");
        assertThat(html).contains("文档列表");
        assertThat(html).contains("当前索引版本");
        assertThat(html).contains("已启用");
        assertThat(html).contains("已删除");
        assertThat(html).contains("重新建立索引");
        assertThat(html).contains("删除文档");
        assertThat(html).contains("查看处理记录");
        assertThat(html).contains("查看文档处理分析");
        assertThat(html).contains("去知识库问答");
        assertThat(html).contains("知识库生命周期说明");
    }

    @Test
    void documentsJsShouldSupportLifecycleOperations() throws Exception {
        String js = fetchUtf8("/documents.js");
        assertThat(js).contains("删除文档");
        assertThat(js).contains("重新建立索引");
        assertThat(js).contains("确认删除");
        assertThat(js).contains("确认重新索引");
        assertThat(js).contains("旧索引不会立即物理删除");
        assertThat(js).contains("历史记录和底层索引数据");
        assertThat(js).contains("删除成功");
        assertThat(js).contains("已提交重新索引任务，请在处理记录中查看进度");
        assertThat(js).contains("重新索引完成，新的文档片段已可用于知识库问答");
        assertThat(js).contains("重新索引失败，系统会保留旧索引继续用于问答");
        assertThat(js).contains("查看技术详情");
        assertThat(js).contains("/documents");
        assertThat(js).contains("/reindex");
    }

    @Test
    void askPageShouldExplainLifecycleFiltering() throws Exception {
        String html = fetchUtf8("/ask.html");
        assertThat(html).contains("已删除文档不会再进入回答引用");
        assertThat(html).contains("最新有效的文档片段");
        assertThat(html).contains("旧版本片段");
        assertThat(html).contains("文档处理分析");
    }

    @Test
    void indexAndAnalyticsEntryShouldRemain() throws Exception {
        String index = fetchUtf8("/index.html");
        assertThat(index).contains("文档处理分析");
        assertThat(index).contains("知识库生命周期管理（V4.0）");

        String analytics = fetchUtf8("/ingestion-analytics.html");
        assertThat(analytics).contains("文档处理分析");
        assertThat(analytics).contains("成功率");
    }

    @Test
    void readmeShouldMentionV40Lifecycle() throws Exception {
        Path readmePath = Path.of("README.md");
        assertThat(Files.exists(readmePath)).isTrue();
        String readme = Files.readString(readmePath, StandardCharsets.UTF_8);
        assertThat(readme).contains("Knowledge Base Lifecycle（V4.0）");
        assertThat(readme).contains("软删除");
        assertThat(readme).contains("重新索引");
    }

    private String fetchUtf8(String path) throws Exception {
        MvcResult result = mockMvc.perform(get(path))
                .andExpect(status().isOk())
                .andReturn();
        return result.getResponse().getContentAsString(StandardCharsets.UTF_8);
    }
}
