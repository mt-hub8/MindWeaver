package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.dto.EmbeddingCacheMetricsResponse;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingCacheMetricsService;
import lombok.RequiredArgsConstructor;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/embedding-cache")
@RequiredArgsConstructor
public class EmbeddingCacheMetricsController {

    private final EmbeddingCacheMetricsService embeddingCacheMetricsService;

    @GetMapping("/metrics")
    public EmbeddingCacheMetricsResponse getMetrics() {
        return embeddingCacheMetricsService.getMetrics();
    }
}
