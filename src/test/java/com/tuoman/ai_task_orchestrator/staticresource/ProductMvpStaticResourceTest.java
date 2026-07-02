package com.tuoman.ai_task_orchestrator.staticresource;

import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.AutoConfigureMockMvc;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.test.web.servlet.MockMvc;

import static org.hamcrest.Matchers.containsString;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.content;
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
    void shouldServeIndexHtmlDirectly() throws Exception {
        mockMvc.perform(get("/index.html"))
                .andExpect(status().isOk())
                .andExpect(content().string(containsString("AI Knowledge Assistant")))
                .andExpect(content().string(containsString("Ask Knowledge Base")))
                .andExpect(content().string(containsString("System Capabilities")));
    }

    @Test
    void shouldServeAskHtml() throws Exception {
        mockMvc.perform(get("/ask.html"))
                .andExpect(status().isOk())
                .andExpect(content().string(containsString("Ask Knowledge Base")))
                .andExpect(content().string(containsString("/ask.js")));
    }

    @Test
    void shouldServeDocumentsHtml() throws Exception {
        mockMvc.perform(get("/documents.html"))
                .andExpect(status().isOk())
                .andExpect(content().string(containsString("No documents have been ingested yet")))
                .andExpect(content().string(containsString("/documents.js")));
    }

    @Test
    void shouldServeEvaluationHtml() throws Exception {
        mockMvc.perform(get("/evaluation.html"))
                .andExpect(status().isOk())
                .andExpect(content().string(containsString("HitRate@K")))
                .andExpect(content().string(containsString("docs/evaluation/reports/")));
    }

    @Test
    void shouldServeRagDemoHtmlAsCompatibilityEntry() throws Exception {
        mockMvc.perform(get("/rag-demo.html"))
                .andExpect(status().isOk())
                .andExpect(content().string(containsString("Ask Knowledge Base")))
                .andExpect(content().string(containsString("/ask.js")));
    }

    @Test
    void shouldServeSharedAppCss() throws Exception {
        mockMvc.perform(get("/app.css"))
                .andExpect(status().isOk())
                .andExpect(content().string(containsString(".app-nav")));
    }
}
