package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.dto.EmbeddingCacheMetricItemResponse;
import com.tuoman.ai_task_orchestrator.dto.EmbeddingCacheMetricsResponse;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingCacheMetricsService;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.test.web.servlet.MockMvc;

import java.util.List;

import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@WebMvcTest(controllers = EmbeddingCacheMetricsController.class)
class EmbeddingCacheMetricsControllerTest {

    @Autowired
    private MockMvc mockMvc;

    @MockitoBean
    private EmbeddingCacheMetricsService embeddingCacheMetricsService;

    @Test
    void shouldReturnEmptyItemsWhenNoMetrics() throws Exception {
        when(embeddingCacheMetricsService.getMetrics()).thenReturn(new EmbeddingCacheMetricsResponse(List.of()));

        mockMvc.perform(get("/embedding-cache/metrics"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.items").isArray())
                .andExpect(jsonPath("$.items.length()").value(0));
    }

    @Test
    void shouldReturnMetricsSnapshot() throws Exception {
        when(embeddingCacheMetricsService.getMetrics()).thenReturn(new EmbeddingCacheMetricsResponse(List.of(
                new EmbeddingCacheMetricItemResponse(
                        "mock",
                        "mock-embedding-v1",
                        128,
                        80L,
                        20L,
                        20L,
                        0L,
                        20L,
                        80L,
                        0.8
                ),
                new EmbeddingCacheMetricItemResponse(
                        "local-worker",
                        "sentence-transformers/all-MiniLM-L6-v2",
                        384,
                        10L,
                        5L,
                        5L,
                        1L,
                        5L,
                        10L,
                        0.6666666666666666
                )
        )));

        mockMvc.perform(get("/embedding-cache/metrics"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.items.length()").value(2))
                .andExpect(jsonPath("$.items[0].provider").value("mock"))
                .andExpect(jsonPath("$.items[0].model").value("mock-embedding-v1"))
                .andExpect(jsonPath("$.items[0].dimension").value(128))
                .andExpect(jsonPath("$.items[0].hitCount").value(80))
                .andExpect(jsonPath("$.items[0].missCount").value(20))
                .andExpect(jsonPath("$.items[0].writeCount").value(20))
                .andExpect(jsonPath("$.items[0].conflictCount").value(0))
                .andExpect(jsonPath("$.items[0].providerCallCount").value(20))
                .andExpect(jsonPath("$.items[0].savedProviderCallCount").value(80))
                .andExpect(jsonPath("$.items[0].hitRate").value(0.8))
                .andExpect(jsonPath("$.items[1].provider").value("local-worker"))
                .andExpect(jsonPath("$.items[1].dimension").value(384));
    }
}
