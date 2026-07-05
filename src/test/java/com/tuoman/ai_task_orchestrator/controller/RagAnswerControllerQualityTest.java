package com.tuoman.ai_task_orchestrator.controller;

import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.AutoConfigureMockMvc;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.http.MediaType;
import org.springframework.test.web.servlet.MockMvc;

import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@SpringBootTest
@AutoConfigureMockMvc
class RagAnswerControllerQualityTest {

    @Autowired
    private MockMvc mockMvc;

    @Test
    void shouldReturnQualityScoreOnNoContextResponse() throws Exception {
        mockMvc.perform(post("/rag/answers")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("""
                                {
                                  "query": "completely unknown topic without indexed docs"
                                }
                                """))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.generation.skipped").value(true))
                .andExpect(jsonPath("$.qualityScore.overallScore").isNumber())
                .andExpect(jsonPath("$.qualityScore.mode").value("BALANCED"))
                .andExpect(jsonPath("$.qualityScore.displayMode").value("平衡模式"))
                .andExpect(jsonPath("$.qualityScore.diagnosis.summary").isNotEmpty())
                .andExpect(jsonPath("$.qualityScore.diagnosis.issues[0].code").value("NO_CONTEXT"));
    }

    @Test
    void shouldDefaultQualityModeToBalanced() throws Exception {
        mockMvc.perform(post("/rag/answers")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"query\":\"no indexed content\"}"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.qualityScore.mode").value("BALANCED"))
                .andExpect(jsonPath("$.qualityScore.weights.retrieval").value(0.30));
    }

    @Test
    void shouldApplyPreciseQualityModeWeights() throws Exception {
        mockMvc.perform(post("/rag/answers")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("""
                                {
                                  "query": "no indexed content",
                                  "qualityMode": "PRECISE"
                                }
                                """))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.qualityScore.mode").value("PRECISE"))
                .andExpect(jsonPath("$.qualityScore.displayMode").value("精准模式"))
                .andExpect(jsonPath("$.qualityScore.weights.context").value(0.30))
                .andExpect(jsonPath("$.qualityScore.weights.answer").value(0.30));
    }

    @Test
    void shouldApplyComprehensiveQualityModeWeights() throws Exception {
        mockMvc.perform(post("/rag/answers")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("""
                                {
                                  "query": "no indexed content",
                                  "qualityMode": "COMPREHENSIVE"
                                }
                                """))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.qualityScore.mode").value("COMPREHENSIVE"))
                .andExpect(jsonPath("$.qualityScore.displayMode").value("全面模式"))
                .andExpect(jsonPath("$.qualityScore.weights.retrieval").value(0.40));
    }
}
