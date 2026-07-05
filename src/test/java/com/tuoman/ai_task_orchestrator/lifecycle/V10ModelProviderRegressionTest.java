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
class V10ModelProviderRegressionTest {

    @Autowired
    private MockMvc mockMvc;

    @Test
    void modelSettingsPageShouldExposeV10ProviderUi() throws Exception {
        String html = fetchUtf8("/model-settings.html");
        assertThat(html).contains("添加模型供应商");
        assertThat(html).contains("测试连接");
        assertThat(html).contains("API Key 不会明文展示");
        assertThat(html).contains("设为默认问答模型");
        assertThat(html).contains("设为默认向量模型");
        assertThat(html).contains("切换 Embedding 模型后，已有文档可能需要重新索引");

        String js = fetchUtf8("/model-settings.js");
        assertThat(js).contains("/model-providers");
        assertThat(js).contains("set-default-llm");
    }

    @Test
    void v10ManualShouldExist() throws Exception {
        Path manual = Path.of("docs/manual/model-provider-settings.md");
        assertThat(Files.exists(manual)).isTrue();
        String content = Files.readString(manual, StandardCharsets.UTF_8);
        assertThat(content).contains("V10.0");
        assertThat(content).contains("API Key");
    }

    @Test
    void readmeShouldMentionModelProviderSettings() throws Exception {
        String readme = Files.readString(Path.of("README.md"), StandardCharsets.UTF_8);
        assertThat(readme).contains("模型供应商");
        assertThat(readme).contains("OpenAI-compatible");
        assertThat(readme).contains("Ollama");
        assertThat(readme).contains("API Key 不会明文展示");
        assertThat(readme).contains("默认测试不依赖真实外部 API");
        assertThat(readme).contains("model-provider-settings.md");
    }

    private String fetchUtf8(String path) throws Exception {
        MvcResult result = mockMvc.perform(get(path))
                .andExpect(status().isOk())
                .andReturn();
        return result.getResponse().getContentAsString(StandardCharsets.UTF_8);
    }
}
