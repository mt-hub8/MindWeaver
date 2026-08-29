package com.tuoman.ai_task_orchestrator.embedding;

import com.tuoman.ai_task_orchestrator.repository.EmbeddingCacheMetricRepository;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.time.LocalDateTime;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatCode;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class EmbeddingCacheMetricsServiceTest {

    @Mock
    private EmbeddingCacheMetricRepository embeddingCacheMetricRepository;

    @InjectMocks
    private EmbeddingCacheMetricsService embeddingCacheMetricsService;

    @Test
    void calculateHitRateShouldReturnZeroWhenNoLookups() {
        assertThat(EmbeddingCacheMetricsService.calculateHitRate(0L, 0L)).isZero();
    }

    @Test
    void calculateHitRateShouldDivideHitByTotalLookups() {
        assertThat(EmbeddingCacheMetricsService.calculateHitRate(80L, 20L)).isEqualTo(0.8);
    }

    @Test
    void shouldSwallowRepositoryFailureWhenRecordingHit() {
        when(embeddingCacheMetricRepository.incrementHit(
                eq(MockEmbeddingClient.PROVIDER),
                eq(MockEmbeddingClient.DEFAULT_MODEL),
                eq(MockEmbeddingClient.DIMENSION),
                any(LocalDateTime.class)
        )).thenThrow(new RuntimeException("database unavailable"));

        assertThatCode(() -> embeddingCacheMetricsService.recordHit(
                MockEmbeddingClient.PROVIDER,
                MockEmbeddingClient.DEFAULT_MODEL,
                MockEmbeddingClient.DIMENSION
        )).doesNotThrowAnyException();
    }
}
