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
 * V9.0 阶段级回归：个人知识工作台产品化收敛验收。
 */
@SpringBootTest
@AutoConfigureMockMvc
class V9PersonalWorkspaceRegressionTest {

    @Autowired
    private MockMvc mockMvc;

    @Test
    void indexShouldExposePersonalWorkspaceBrandingAndCoreCards() throws Exception {
        String index = fetchUtf8("/index.html");
        assertThat(index).contains("个人 AI 知识工作台");
        assertThat(index).contains("Personal AI Knowledge Workspace");
        assertThat(index).contains("构建个人知识库");
        assertThat(index).contains("配置 AI 模型");
        assertThat(index).contains("基于知识库提问");
        assertThat(index).contains("创建 AI 任务报告");
    }

    @Test
    void unifiedNavShouldExistOnKeyPages() throws Exception {
        String appNav = fetchUtf8("/app-nav.js");
        assertThat(appNav).contains("模型设置");
        assertThat(appNav).contains("系统设置");
        assertThat(appNav).contains("AI 任务");

        String documents = fetchUtf8("/documents.html");
        assertThat(documents).contains("app-nav-mount");

        String documentsJs = fetchUtf8("/documents.js");
        assertThat(documentsJs).contains("查看技术详情");
    }

    @Test
    void modelSettingsPageShouldExist() throws Exception {
        String html = fetchUtf8("/model-settings.html");
        assertThat(html).contains("模型设置");
        assertThat(html).contains("测试连接");
        assertThat(html).contains("当前模型");

        String js = fetchUtf8("/model-settings.js");
        assertThat(js).contains("/model-providers");
    }

    @Test
    void settingsPageShouldExist() throws Exception {
        String html = fetchUtf8("/settings.html");
        assertThat(html).contains("系统设置");
        assertThat(html).contains("本地数据目录");
        assertThat(html).contains("当前运行模式");
        assertThat(html).contains("PersonalAIKnowledgeWorkspace");
    }

    @Test
    void askPageShouldFoldTechnicalDetails() throws Exception {
        String html = fetchUtf8("/ask.html");
        assertThat(html).contains("查看技术详情");

        String js = fetchUtf8("/ask.js");
        assertThat(js).contains("查看技术详情");
    }

    @Test
    void agentTasksPageShouldFoldTechnicalDetails() throws Exception {
        String html = fetchUtf8("/agent-tasks.html");
        assertThat(html).contains("查看技术详情");
    }

    @Test
    void windowsScriptsShouldExist() throws Exception {
        assertThat(Files.exists(Path.of("scripts/windows/check-env.ps1"))).isTrue();
        assertThat(Files.exists(Path.of("scripts/windows/start-local.ps1"))).isTrue();
        assertThat(Files.exists(Path.of("scripts/windows/README.md"))).isTrue();
    }

    @Test
    void v9ManualShouldExist() throws Exception {
        Path manual = Path.of("docs/manual/local-personal-knowledge-workspace.md");
        assertThat(Files.exists(manual)).isTrue();
        String content = Files.readString(manual, StandardCharsets.UTF_8);
        assertThat(content).contains("V9.0");
        assertThat(content).contains("个人 AI 知识工作台");
    }

    @Test
    void readmeShouldMentionV9PersonalWorkspace() throws Exception {
        String readme = Files.readString(Path.of("README.md"), StandardCharsets.UTF_8);
        assertThat(readme).contains("Personal AI Knowledge Workspace");
        assertThat(readme).contains("个人 AI 知识工作台");
        assertThat(readme).contains("本地优先");
        assertThat(readme).contains("默认 mock");
        assertThat(readme).contains("local-ai");
        assertThat(readme).contains("Ollama");
        assertThat(readme).contains("Java + Python");
        assertThat(readme).contains("当前不做");
        assertThat(readme).contains("后续路线");
        assertThat(readme).contains("local-personal-knowledge-workspace.md");
    }

    private String fetchUtf8(String path) throws Exception {
        MvcResult result = mockMvc.perform(get(path))
                .andExpect(status().isOk())
                .andReturn();
        return result.getResponse().getContentAsString(StandardCharsets.UTF_8);
    }
}
