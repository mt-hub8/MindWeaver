package com.tuoman.ai_task_orchestrator.lifecycle;

import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.AutoConfigureMockMvc;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.test.web.servlet.MockMvc;
import org.springframework.test.web.servlet.MvcResult;

import java.nio.charset.StandardCharsets;

import static org.assertj.core.api.Assertions.assertThat;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@SpringBootTest
@AutoConfigureMockMvc
class FrontendProductizationRegressionTest {

    @Autowired
    private MockMvc mockMvc;

    @Test
    void indexShouldExposeProductHome() throws Exception {
        String index = fetchUtf8("/index.html");
        assertThat(index).contains("MindWeaver");
        assertThat(index).contains("mw-page");
        assertThat(index).contains("mindweaver.css");
        assertThat(index).contains("个人 AI 知识工作台");
        assertThat(index).contains("构建个人知识库");
        assertThat(index).contains("配置 AI 模型");
        assertThat(index).contains("基于知识库提问");
        assertThat(index).contains("创建 AI 任务报告");
        assertThat(index).contains("app-shell");
        assertThat(index).contains("feature-grid-2x2");
        assertThat(index).contains("content-container");
    }

    @Test
    void sidebarNavShouldListAllPrimaryEntries() throws Exception {
        String appNav = fetchUtf8("/app-nav.js");
        assertThat(appNav).contains("MindWeaver");
        assertThat(appNav).contains("总览");
        assertThat(appNav).contains("文档管理");
        assertThat(appNav).contains("知识库分组");
        assertThat(appNav).contains("知识库问答");
        assertThat(appNav).contains("AI 任务");
        assertThat(appNav).contains("模型设置");
        assertThat(appNav).contains("质量诊断");
        assertThat(appNav).contains("垃圾箱");
        assertThat(appNav).contains("系统设置");
        assertThat(appNav).contains("使用指南");
    }

    @Test
    void pagesShouldUseAppShellStructure() throws Exception {
        for (String path : new String[]{
                "/index.html", "/documents.html", "/collections.html", "/ask.html", "/model-settings.html",
                "/agent-tasks.html", "/trash.html", "/settings.html", "/quality.html", "/guide.html"
        }) {
            String html = fetchUtf8(path);
            assertThat(html).contains("mw-page");
            assertThat(html).contains("mindweaver.css");
        assertThat(html).contains("mw-app-shell");
            assertThat(html).contains("app-shell");
            assertThat(html).contains("app-sidebar-mount");
            assertThat(html).contains("main-panel");
            assertThat(html).contains("content-container");
        }
    }

    @Test
    void collectionsPageShouldExposeV127LayoutCopy() throws Exception {
        String html = fetchUtf8("/collections.html");
        assertThat(html).contains("知识库分组");
        assertThat(html).contains("新建知识库分组");
        assertThat(html).contains("还没有知识库分组");
        assertThat(html).contains("新建分组");
        assertThat(html).contains("form-grid");
        assertThat(html).contains("entity-grid");
        assertThat(html).contains("查看技术详情");
    }

    @Test
    void modelSettingsShouldMatchReferenceLayoutCopy() throws Exception {
        String html = fetchUtf8("/model-settings.html");
        assertThat(html).contains("模型设置");
        assertThat(html).contains("当前模型");
        assertThat(html).contains("模型提供商");
        assertThat(html).contains("添加提供商");
        assertThat(html).contains("查看技术详情");
        assertThat(html).contains("测试连接");
    }

    @Test
    void primaryPagesShouldExposeTitlesAndTechnicalDetails() throws Exception {
        assertThat(fetchUtf8("/documents.html")).contains("文档管理").contains("查看技术详情");
        assertThat(fetchUtf8("/ask.html")).contains("知识库问答").contains("查看技术详情");
        assertThat(fetchUtf8("/agent-tasks.html")).contains("AI 任务").contains("查看技术详情");
        assertThat(fetchUtf8("/trash.html")).contains("垃圾箱");
        assertThat(fetchUtf8("/settings.html")).contains("系统设置").contains("清理缓存");
        assertThat(fetchUtf8("/guide.html")).contains("使用指南");
    }

    @Test
    void designSystemCssShouldDefineWarmIvoryTokens() throws Exception {
        String css = fetchUtf8("/mindweaver.css");
        assertThat(css).contains("--mw-page-bg: #F5EEDD");
        assertThat(css).contains("--mw-accent: #B8822A");
        assertThat(css).contains(".mw-app-shell");
        assertThat(css).contains(".mw-sidebar");
        assertThat(css).contains(".entity-card");
        assertThat(css).contains(".form-grid");
        assertThat(css).contains(".content-container");
        assertThat(css).contains(".hero-card");
        assertThat(css).contains(".technical-details");
        assertThat(css).contains(".setting-row");
        assertThat(css).contains(".danger-button");
        assertThat(css).contains(".list-card");
        assertThat(css).contains(".provider-card");

        String compat = fetchUtf8("/app-shell.css");
        assertThat(compat).contains("--app-bg: #f7f2ea");
        assertThat(compat).contains("--panel-bg: #fffdf8");
        assertThat(compat).contains("--content-max-width: 1180px");
    }

    private String fetchUtf8(String path) throws Exception {
        MvcResult result = mockMvc.perform(get(path))
                .andExpect(status().isOk())
                .andReturn();
        return result.getResponse().getContentAsString(StandardCharsets.UTF_8);
    }
}
