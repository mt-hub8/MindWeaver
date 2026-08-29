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
class KnowledgeBaseAgentTaskRegressionTest {

    @Autowired
    private MockMvc mockMvc;

    @Test
    void manualDocumentShouldExist() throws Exception {
        Path manualPath = Path.of("docs/manual/ai-runtime-and-agent-task-orchestration.md");
        assertThat(Files.exists(manualPath)).isTrue();
        String content = Files.readString(manualPath, StandardCharsets.UTF_8);
        assertThat(content).contains("V6.0");
        assertThat(content).contains("Java + Python");
        assertThat(content).contains("local-ai");
        assertThat(content).contains("Agent Task");
    }

    @Test
    void agentTasksPageShouldExposeChineseUi() throws Exception {
        String html = fetchUtf8("/agent-tasks.html");
        assertThat(html).contains("AI 任务");
        assertThat(html).contains("新建 AI 任务");
        assertThat(html).contains("任务目标");
        assertThat(html).contains("知识库范围");
        assertThat(html).contains("执行步骤");
        assertThat(html).contains("工具执行记录");
        assertThat(html).contains("任务结果");
        assertThat(html).contains("引用来源");
        assertThat(html).contains("查看执行过程");

        String js = fetchUtf8("/agent-tasks.js");
        assertThat(js).contains("全部文档");
        assertThat(js).contains("/agent/tasks");
        assertThat(js).contains("查看技术详情");
        assertThat(js).contains("工具输入");
        assertThat(html).contains("模型调用摘要");
        assertThat(html).contains("指定知识库分组");
        assertThat(html).contains("当前可用工具");
    }

    @Test
    void agentToolsPageShouldExposeChineseUi() throws Exception {
        String html = fetchUtf8("/agent-tools.html");
        assertThat(html).contains("可用工具");
        assertThat(html).contains("检索知识库");
        assertThat(html).contains("总结检索结果");
        assertThat(html).contains("当前阶段仅支持内置安全工具");
    }

    @Test
    void manualV70DocumentShouldExist() throws Exception {
        Path manualPath = Path.of("docs/manual/tool-using-agent-workflow.md");
        assertThat(Files.exists(manualPath)).isTrue();
        String content = Files.readString(manualPath, StandardCharsets.UTF_8);
        assertThat(content).contains("V7.0");
        assertThat(content).contains("knowledge_search");
        assertThat(content).contains("固定");
    }

    @Test
    void indexShouldExposeAgentTaskEntry() throws Exception {
        String index = fetchUtf8("/index.html");
        assertThat(index).contains("创建 AI 任务报告");
        assertThat(index).contains("/agent-tasks.html");
    }

    @Test
    void askPageShouldMentionAgentTasks() throws Exception {
        String html = fetchUtf8("/ask.html");
        assertThat(html).contains("AI 任务");
    }

    @Test
    void readmeShouldMentionV70() throws Exception {
        String readme = Files.readString(Path.of("README.md"), StandardCharsets.UTF_8);
        assertThat(readme).contains("V7.0");
        assertThat(readme).contains("/agent-tasks.html");
        assertThat(readme).contains("tool-using-agent-workflow.md");
    }

    @Test
    void readmeShouldMentionV80LocalAi() throws Exception {
        String readme = Files.readString(Path.of("README.md"), StandardCharsets.UTF_8);
        assertThat(readme).contains("V8.0");
        assertThat(readme).contains("real-local-ai-runtime-with-ollama.md");
    }

    @Test
    void manualV80DocumentShouldExist() throws Exception {
        Path manualPath = Path.of("docs/manual/real-local-ai-runtime-with-ollama.md");
        assertThat(Files.exists(manualPath)).isTrue();
        String content = Files.readString(manualPath, StandardCharsets.UTF_8);
        assertThat(content).contains("Ollama");
        assertThat(content).contains("qwen3-embedding");
        assertThat(content).contains("local-ai");
        assertThat(content).contains("ollama pull");
    }

    private String fetchUtf8(String path) throws Exception {
        MvcResult result = mockMvc.perform(get(path))
                .andExpect(status().isOk())
                .andReturn();
        return result.getResponse().getContentAsString(StandardCharsets.UTF_8);
    }
}
