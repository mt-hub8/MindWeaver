package com.tuoman.ai_task_orchestrator.controller;

import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.AutoConfigureMockMvc;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.test.web.servlet.MockMvc;

import static org.hamcrest.Matchers.containsString;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@SpringBootTest
@AutoConfigureMockMvc
class RuntimeStatusControllerTest {

    @Autowired
    private MockMvc mockMvc;

    @Test
    void statusShouldReturnMockRuntimeUnderDefaultTestProfile() throws Exception {
        mockMvc.perform(get("/runtime/status"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.embeddingProvider").value("mock"))
                .andExpect(jsonPath("$.llmProvider").value("mock"))
                .andExpect(jsonPath("$.statusMessage", containsString("mock")))
                .andExpect(jsonPath("$.pythonWorkerReachable").value(false))
                .andExpect(jsonPath("$.ollamaReachable").value(false));
    }

    @Test
    void testEmbeddingShouldSucceedWithMockProvider() throws Exception {
        mockMvc.perform(post("/runtime/test/embedding"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.success").value(true))
                .andExpect(jsonPath("$.provider").value("mock"));
    }

    @Test
    void testLlmShouldSucceedWithMockProvider() throws Exception {
        mockMvc.perform(post("/runtime/test/llm"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.success").value(true))
                .andExpect(jsonPath("$.provider").value("mock"));
    }
}
