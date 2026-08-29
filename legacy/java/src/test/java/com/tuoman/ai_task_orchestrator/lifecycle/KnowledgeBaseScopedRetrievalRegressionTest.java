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
 * V5.0 阶段级回归：知识库分组 UI、范围检索 manual 与跨能力收敛验收。
 */
@SpringBootTest
@AutoConfigureMockMvc
class KnowledgeBaseScopedRetrievalRegressionTest {

    @Autowired
    private MockMvc mockMvc;

    @Test
    void manualDocumentShouldExistWithScopedRetrievalSections() throws Exception {
        Path manualPath = Path.of("docs/manual/scoped-retrieval-and-collections.md");
        assertThat(Files.exists(manualPath)).isTrue();
        String content = Files.readString(manualPath, StandardCharsets.UTF_8);
        assertThat(content).contains("V5.0");
        assertThat(content).contains("知识库分组");
        assertThat(content).contains("范围检索");
        assertThat(content).contains("应用层过滤");
        assertThat(content).contains("V4.0");
    }

    @Test
    void collectionsPageShouldExposeChineseUi() throws Exception {
        String html = fetchUtf8("/collections.html");
        assertThat(html).contains("知识库分组");
        assertThat(html).contains("新建分组");
        assertThat(html).contains("分组名称");
        assertThat(html).contains("文档数量");
        assertThat(html).contains("可用于问答的文档");
        assertThat(html).contains("该分组暂无文档");
        assertThat(html).contains("查看技术详情");

        String js = fetchUtf8("/collections.js");
        assertThat(js).contains("/collections");
        assertThat(js).contains("移出分组");
        assertThat(js).contains("前往该分组问答");
        assertThat(js).contains("创建成功");
    }

    @Test
    void documentsPageShouldSupportCollectionMembership() throws Exception {
        String html = fetchUtf8("/documents.html");
        assertThat(html).contains("所属分组");
        assertThat(html).contains("知识库分组");

        String js = fetchUtf8("/documents.js");
        assertThat(js).contains("加入分组");
        assertThat(js).contains("移出分组");
        assertThat(js).contains("选择知识库分组");
        assertThat(js).contains("垃圾箱中的文档可保留分组归属，但不会参与问答");
    }

    @Test
    void askPageShouldSupportScopeSelection() throws Exception {
        String html = fetchUtf8("/ask.html");
        assertThat(html).contains("问答范围");
        assertThat(html).contains("全部文档");

        String js = fetchUtf8("/ask.js");
        assertThat(js).contains("仅在该分组中提问");
        assertThat(js).contains("collectionId");
        assertThat(js).contains("当前问答范围");
    }

    @Test
    void indexShouldExposeCollectionsEntry() throws Exception {
        String index = fetchUtf8("/index.html");
        assertThat(index).contains("知识库分组");
        assertThat(index).contains("/collections.html");
        assertThat(index).contains("范围检索");
    }

    @Test
    void readmeShouldMentionV50Collections() throws Exception {
        String readme = Files.readString(Path.of("README.md"), StandardCharsets.UTF_8);
        assertThat(readme).contains("V5.0");
        assertThat(readme).contains("知识库分组");
        assertThat(readme).contains("/collections.html");
        assertThat(readme).contains("collectionId");
    }

    private String fetchUtf8(String path) throws Exception {
        MvcResult result = mockMvc.perform(get(path))
                .andExpect(status().isOk())
                .andReturn();
        return result.getResponse().getContentAsString(StandardCharsets.UTF_8);
    }
}
