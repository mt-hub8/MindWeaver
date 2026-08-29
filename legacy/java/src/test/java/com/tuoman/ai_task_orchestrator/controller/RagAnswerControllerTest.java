package com.tuoman.ai_task_orchestrator.controller;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.tuoman.ai_task_orchestrator.common.error.GlobalExceptionHandler;
import com.tuoman.ai_task_orchestrator.dto.RagAnswerRequest;
import com.tuoman.ai_task_orchestrator.dto.RagAnswerResponse;
import com.tuoman.ai_task_orchestrator.dto.RagCitationResponse;
import com.tuoman.ai_task_orchestrator.dto.RagGenerationMetadataResponse;
import com.tuoman.ai_task_orchestrator.dto.RagRetrievalMetadataResponse;
import com.tuoman.ai_task_orchestrator.service.RagAnswerService;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.context.annotation.Import;
import org.springframework.http.MediaType;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.test.web.servlet.MockMvc;

import java.util.List;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@WebMvcTest(controllers = RagAnswerController.class)
@Import(GlobalExceptionHandler.class)
class RagAnswerControllerTest {

    @Autowired
    private MockMvc mockMvc;

    @Autowired
    private ObjectMapper objectMapper;

    @MockitoBean
    private RagAnswerService ragAnswerService;

    @Test
    void shouldReturnRagAnswerWithMetadata() throws Exception {
        when(ragAnswerService.answer(any(RagAnswerRequest.class))).thenReturn(new RagAnswerResponse(
                "根据检索到的上下文，问题与以下来源相关：[1]",
                List.of(new RagCitationResponse(1, 10L, 101L, 0.91, "cache key chunk hash")),
                new RagRetrievalMetadataResponse(5, 1, "mock", "mock-embedding-v1", 128, "ExactCosineVectorStore"),
                new RagGenerationMetadataResponse(
                        "local-ollama",
                        "qwen2.5:7b",
                        "local-ollama",
                        "qwen2.5:7b",
                        null,
                        null,
                        null,
                        null,
                        null
                ),
                null
        ));

        mockMvc.perform(post("/rag/answers")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("""
                                {
                                  "query": "V2.6.8 Embedding Cache 的 cache key 为什么不能只用 chunkHash？",
                                  "topK": 5
                                }
                                """))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.answer").isNotEmpty())
                .andExpect(jsonPath("$.citations[0].sourceIndex").value(1))
                .andExpect(jsonPath("$.citations[0].documentId").value(10))
                .andExpect(jsonPath("$.citations[0].chunkId").value(101))
                .andExpect(jsonPath("$.citations[0].score").value(0.91))
                .andExpect(jsonPath("$.citations[0].contentSnippet").value("cache key chunk hash"))
                .andExpect(jsonPath("$.retrieval.topK").value(5))
                .andExpect(jsonPath("$.retrieval.returned").value(1))
                .andExpect(jsonPath("$.retrieval.provider").value("mock"))
                .andExpect(jsonPath("$.retrieval.model").value("mock-embedding-v1"))
                .andExpect(jsonPath("$.retrieval.dimension").value(128))
                .andExpect(jsonPath("$.retrieval.vectorStore").value("ExactCosineVectorStore"))
                .andExpect(jsonPath("$.generation.provider").value("local-ollama"))
                .andExpect(jsonPath("$.generation.model").value("qwen2.5:7b"))
                .andExpect(jsonPath("$.generation.llmProvider").value("local-ollama"))
                .andExpect(jsonPath("$.generation.llmModel").value("qwen2.5:7b"));
    }

    @Test
    void shouldReturnValidationErrorForBlankQuery() throws Exception {
        mockMvc.perform(post("/rag/answers")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"query\":\"   \"}"))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("VALIDATION_ERROR"));
    }

    @Test
    void shouldReturnValidationErrorForTopKBelowOne() throws Exception {
        mockMvc.perform(post("/rag/answers")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("""
                                {
                                  "query": "test",
                                  "topK": 0
                                }
                                """))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("VALIDATION_ERROR"));
    }

    @Test
    void shouldReturnValidationErrorForTopKAboveMax() throws Exception {
        mockMvc.perform(post("/rag/answers")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("""
                                {
                                  "query": "test",
                                  "topK": 11
                                }
                                """))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("VALIDATION_ERROR"));
    }

    @Test
    void shouldReturnNoContextAnswer() throws Exception {
        when(ragAnswerService.answer(any(RagAnswerRequest.class))).thenReturn(new RagAnswerResponse(
                "根据当前检索到的文档内容，无法确定。",
                List.of(),
                new RagRetrievalMetadataResponse(5, 0, "mock", "mock-embedding-v1", 128, "ExactCosineVectorStore"),
                new RagGenerationMetadataResponse(null, null, null, null, true, "NO_RETRIEVED_CONTEXT", null, null, null),
                null
        ));

        mockMvc.perform(post("/rag/answers")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"query\":\"unknown\"}"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.citations").isArray())
                .andExpect(jsonPath("$.citations.length()").value(0))
                .andExpect(jsonPath("$.generation.skipped").value(true))
                .andExpect(jsonPath("$.generation.reason").value("NO_RETRIEVED_CONTEXT"));
    }
}
