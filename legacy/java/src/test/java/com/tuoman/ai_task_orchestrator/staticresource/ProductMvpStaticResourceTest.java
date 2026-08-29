package com.tuoman.ai_task_orchestrator.staticresource;

import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.AutoConfigureMockMvc;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.test.web.servlet.MockMvc;
import org.springframework.test.web.servlet.MvcResult;

import java.nio.charset.StandardCharsets;

import static org.assertj.core.api.Assertions.assertThat;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.forwardedUrl;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@SpringBootTest
@AutoConfigureMockMvc
class ProductMvpStaticResourceTest {

    @Autowired
    private MockMvc mockMvc;

    @Test
    void shouldServeIndexHtmlAtRoot() throws Exception {
        mockMvc.perform(get("/"))
                .andExpect(status().isOk())
                .andExpect(forwardedUrl("/index.html"));
    }

    @Test
    void shouldServeIndexHtmlWithChineseProductPath() throws Exception {
        String content = fetchUtf8("/index.html");
        assertThat(content).contains("个人 AI 知识工作台");
        assertThat(content).contains("知识库问答");
        assertThat(content).contains("文档管理");
        assertThat(content).contains("文档处理分析");
    }

    @Test
    void shouldServeAskHtmlWithChineseUploadToAskFlow() throws Exception {
        String content = fetchUtf8("/ask.html");
        assertThat(content).contains("知识库问答");
        assertThat(content).contains("去上传文档");
        assertThat(content).contains("检索引用（Citation）");
        assertThat(content).contains("/ask.js");
    }

    @Test
    void shouldServeAskHtmlWithGroundedAnswerDiagnosticsCopy() throws Exception {
        String html = fetchUtf8("/ask.html");
        String js = fetchUtf8("/ask.js");

        assertThat(html).contains("可信回答状态");
        assertThat(html).contains("引用校验");
        assertThat(html).contains("未被资料支持的主张");
        assertThat(html).contains("当前资料不足");
        assertThat(html).contains("查看技术详情");
        assertThat(js).contains("引用已校验");
        assertThat(js).contains("引用支撑较弱");
        assertThat(js).contains("存在未被资料支持的主张");
    }

    @Test
    void shouldServeDocumentsHtmlWithChineseUploadSection() throws Exception {
        String content = fetchUtf8("/documents.html");
        assertThat(content).contains("文档管理");
        assertThat(content).contains("上传文档");
        assertThat(content).contains(".pdf");
    }

    @Test
    void shouldServeAskHtmlWithDeletedDocumentHint() throws Exception {
        String content = fetchUtf8("/ask.html");
        assertThat(content).contains("垃圾箱中的文档不会再进入回答引用");
        assertThat(content).contains("旧版本片段");
        assertThat(content).contains("知识库生命周期过滤说明");
    }

    @Test
    void shouldServeAskHtmlWithSupersededChunkHint() throws Exception {
        String content = fetchUtf8("/ask.html");
        assertThat(content).contains("旧版本片段");
        assertThat(content).contains("最新有效的文档片段");
    }

    @Test
    void shouldServeDocumentsJsWithReindexActions() throws Exception {
        String content = fetchUtf8("/documents.js");
        assertThat(content).contains("重新建立索引");
        assertThat(content).contains("确认重新索引");
        assertThat(content).contains("旧索引不会立即物理删除");
        assertThat(content).contains("请在处理记录中查看进度");
        assertThat(content).contains("/reindex");
    }

    @Test
    void shouldServeDocumentsJsWithDeleteActions() throws Exception {
        String content = fetchUtf8("/documents.js");
        assertThat(content).contains("删除文档");
        assertThat(content).contains("确认删除");
        assertThat(content).contains("垃圾箱");
        assertThat(content).contains("method: \"DELETE\"");
    }

    @Test
    void shouldServeDocumentsHtmlWithLifecycleColumn() throws Exception {
        String content = fetchUtf8("/documents.html");
        assertThat(content).contains("文档状态");
    }

    @Test
    void shouldServeDocumentsJsWithIngestionEndpoints() throws Exception {
        String content = fetchUtf8("/documents.js");
        assertThat(content).contains("/documents/upload");
        assertThat(content).contains("/documents/ingestions");
        assertThat(content).contains("重新处理");
        assertThat(content).contains("查看处理记录");
        assertThat(content).contains("技术详情");
    }

    @Test
    void shouldServeDocumentsHtmlWithLifecycleManagement() throws Exception {
        String content = fetchUtf8("/documents.html");
        assertThat(content).contains("文档列表");
        assertThat(content).contains("当前索引版本");
        assertThat(content).contains("已启用");
        assertThat(content).contains("垃圾箱");
        assertThat(content).contains("查看文档处理分析");
        assertThat(content).contains("知识库生命周期说明");
    }

    @Test
    void shouldServeDocumentsHtmlWithChineseEmptyState() throws Exception {
        String content = fetchUtf8("/documents.html");
        assertThat(content).contains("还没有上传任何文档");
        assertThat(content).contains("处理记录");
        assertThat(content).contains("任务时间线");
        assertThat(content).contains("/documents.js");
    }

    @Test
    void shouldServeEvaluationHtml() throws Exception {
        String content = fetchUtf8("/evaluation.html");
        assertThat(content).contains("HitRate@K");
        assertThat(content).contains("docs/evaluation/reports/");
    }

    @Test
    void shouldServeRagDemoHtmlAsCompatibilityEntry() throws Exception {
        String content = fetchUtf8("/rag-demo.html");
        assertThat(content).contains("知识库问答");
        assertThat(content).contains("/ask.js");
    }

    @Test
    void shouldServeIngestionAnalyticsHtmlWithChineseContent() throws Exception {
        String content = fetchUtf8("/ingestion-analytics.html");
        assertThat(content).contains("文档处理分析");
        assertThat(content).contains("成功率");
        assertThat(content).contains("常见失败原因");
        assertThat(content).contains("处理较慢的任务");
        assertThat(content).contains("/ingestion-analytics.js");
    }

    @Test
    void shouldServeIngestionAnalyticsJsWithAnalyticsEndpoint() throws Exception {
        String content = fetchUtf8("/ingestion-analytics.js");
        assertThat(content).contains("/documents/ingestions/analytics");
        assertThat(content).contains("最近 24 小时");
    }

    @Test
    void shouldServeIndexHtmlWithAnalyticsEntry() throws Exception {
        String content = fetchUtf8("/index.html");
        assertThat(content).contains("文档处理分析");
    }

    @Test
    void shouldServeSharedAppCss() throws Exception {
        String content = fetchUtf8("/app.css");
        assertThat(content).contains(".app-nav");
    }

    private String fetchUtf8(String path) throws Exception {
        MvcResult result = mockMvc.perform(get(path))
                .andExpect(status().isOk())
                .andReturn();
        return result.getResponse().getContentAsString(StandardCharsets.UTF_8);
    }
}
