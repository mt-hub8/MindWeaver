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
class V11RagQualityRegressionTest {

    @Autowired
    private MockMvc mockMvc;

    @Test
    void askPageShouldExposeQualityScoreUiCopy() throws Exception {
        String html = fetchUtf8("/ask.html");
        assertThat(html).contains("综合评分");
        assertThat(html).contains("检索质量");
        assertThat(html).contains("上下文质量");
        assertThat(html).contains("回答质量");
        assertThat(html).contains("引用质量");
        assertThat(html).contains("主要扣分原因");
        assertThat(html).contains("优化建议");
        assertThat(html).contains("平衡模式");
        assertThat(html).contains("精准模式");
        assertThat(html).contains("全面模式");
        assertThat(html).contains("查看技术详情");

        String js = fetchUtf8("/ask.js");
        assertThat(js).contains("qualityMode");
        assertThat(js).contains("qualityScore");
    }

    @Test
    void manualAndReadmeShouldDocumentRagQualityScore() throws Exception {
        Path manual = Path.of("docs/manual/rag-quality-score-and-diagnostics.md");
        assertThat(Files.exists(manual)).isTrue();
        String manualContent = Files.readString(manual, StandardCharsets.UTF_8);
        assertThat(manualContent).contains("V11.0");
        assertThat(manualContent).contains("平衡模式");
        assertThat(manualContent).contains("启发式");

        String readme = Files.readString(Path.of("README.md"), StandardCharsets.UTF_8);
        assertThat(readme).contains("RAG 质量评分");
        assertThat(readme).contains("综合评分");
        assertThat(readme).contains("优化建议");
        assertThat(readme).contains("默认测试不依赖真实外部 LLM");
    }

    private String fetchUtf8(String path) throws Exception {
        MvcResult result = mockMvc.perform(get(path))
                .andExpect(status().isOk())
                .andReturn();
        return result.getResponse().getContentAsString(StandardCharsets.UTF_8);
    }
}
