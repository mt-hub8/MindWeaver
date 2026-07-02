package com.tuoman.ai_task_orchestrator.staticresource;

import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.AutoConfigureMockMvc;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.test.web.servlet.MockMvc;

import static org.hamcrest.Matchers.containsString;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.content;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@SpringBootTest
@AutoConfigureMockMvc
class RagDemoStaticResourceTest {

    @Autowired
    private MockMvc mockMvc;

    @Test
    void shouldServeRagDemoHtml() throws Exception {
        mockMvc.perform(get("/rag-demo.html"))
                .andExpect(status().isOk())
                .andExpect(content().string(containsString("RAG Answer Demo")))
                .andExpect(content().string(containsString("/rag-demo.js")));
    }

    @Test
    void shouldServeRagDemoAssets() throws Exception {
        mockMvc.perform(get("/rag-demo.css"))
                .andExpect(status().isOk())
                .andExpect(content().string(containsString(".page")));

        mockMvc.perform(get("/rag-demo.js"))
                .andExpect(status().isOk())
                .andExpect(content().string(containsString("/rag/answers")));
    }
}
