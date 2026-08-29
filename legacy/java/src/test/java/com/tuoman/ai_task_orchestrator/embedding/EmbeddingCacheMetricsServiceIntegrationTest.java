package com.tuoman.ai_task_orchestrator.embedding;

import com.tuoman.ai_task_orchestrator.dto.EmbeddingCacheMetricItemResponse;
import com.tuoman.ai_task_orchestrator.repository.EmbeddingCacheMetricRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.annotation.Transactional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatCode;

@SpringBootTest
@Transactional
class EmbeddingCacheMetricsServiceIntegrationTest {

    @Autowired
    private EmbeddingCacheMetricsService embeddingCacheMetricsService;

    @Autowired
    private EmbeddingCacheMetricRepository embeddingCacheMetricRepository;

    private String testProvider;

    @BeforeEach
    void setUpUniqueProvider() {
        testProvider = "metrics-it-" + System.nanoTime();
    }

    @Test
    void shouldRecordHitAndSavedProviderCallCount() {
        embeddingCacheMetricsService.recordHit(testProvider, MockEmbeddingClient.DEFAULT_MODEL, 128);
        embeddingCacheMetricsService.recordHit(testProvider, MockEmbeddingClient.DEFAULT_MODEL, 128);

        EmbeddingCacheMetricItemResponse item = metricItem(testProvider, MockEmbeddingClient.DEFAULT_MODEL, 128);
        assertThat(item.getHitCount()).isEqualTo(2L);
        assertThat(item.getSavedProviderCallCount()).isEqualTo(2L);
        assertThat(item.getHitRate()).isEqualTo(1.0);
    }

    @Test
    void shouldRecordMissAndProviderCallCount() {
        embeddingCacheMetricsService.recordMiss(testProvider, MockEmbeddingClient.DEFAULT_MODEL, 128);

        EmbeddingCacheMetricItemResponse item = metricItem(testProvider, MockEmbeddingClient.DEFAULT_MODEL, 128);
        assertThat(item.getMissCount()).isEqualTo(1L);
        assertThat(item.getProviderCallCount()).isEqualTo(1L);
        assertThat(item.getWriteCount()).isZero();
        assertThat(item.getHitRate()).isZero();
    }

    @Test
    void shouldRecordWriteAfterMiss() {
        embeddingCacheMetricsService.recordMiss(testProvider, MockEmbeddingClient.DEFAULT_MODEL, 128);
        embeddingCacheMetricsService.recordWrite(testProvider, MockEmbeddingClient.DEFAULT_MODEL, 128);

        EmbeddingCacheMetricItemResponse item = metricItem(testProvider, MockEmbeddingClient.DEFAULT_MODEL, 128);
        assertThat(item.getMissCount()).isEqualTo(1L);
        assertThat(item.getWriteCount()).isEqualTo(1L);
    }

    @Test
    void shouldRecordConflictCount() {
        embeddingCacheMetricsService.recordConflict(testProvider, MockEmbeddingClient.DEFAULT_MODEL, 128);

        assertThat(metricItem(testProvider, MockEmbeddingClient.DEFAULT_MODEL, 128).getConflictCount()).isEqualTo(1L);
    }

    @Test
    void shouldCalculateHitRateCorrectly() {
        embeddingCacheMetricsService.recordHit(testProvider, MockEmbeddingClient.DEFAULT_MODEL, 128);
        embeddingCacheMetricsService.recordHit(testProvider, MockEmbeddingClient.DEFAULT_MODEL, 128);
        embeddingCacheMetricsService.recordHit(testProvider, MockEmbeddingClient.DEFAULT_MODEL, 128);
        embeddingCacheMetricsService.recordMiss(testProvider, MockEmbeddingClient.DEFAULT_MODEL, 128);

        assertThat(metricItem(testProvider, MockEmbeddingClient.DEFAULT_MODEL, 128).getHitRate()).isEqualTo(0.75);
    }

    @Test
    void shouldReturnZeroHitRateWhenNoLookups() {
        EmbeddingCacheMetricItemResponse item = new EmbeddingCacheMetricItemResponse(
                MockEmbeddingClient.PROVIDER,
                MockEmbeddingClient.DEFAULT_MODEL,
                128,
                0L,
                0L,
                0L,
                0L,
                0L,
                0L,
                EmbeddingCacheMetricsService.calculateHitRate(0L, 0L)
        );

        assertThat(item.getHitRate()).isZero();
    }

    @Test
    void shouldTrackDifferentEmbeddingSpacesSeparately() {
        String providerA = testProvider + "-a";
        String providerB = testProvider + "-b";
        embeddingCacheMetricsService.recordHit(providerA, "model-a", 128);
        embeddingCacheMetricsService.recordMiss(providerB, "model-b", 256);

        assertThat(metricItem(providerA, "model-a", 128).getHitCount()).isEqualTo(1L);
        assertThat(metricItem(providerB, "model-b", 256).getMissCount()).isEqualTo(1L);
    }

    @Test
    void shouldReturnEmptyItemsWhenNoMetrics() {
        assertThat(embeddingCacheMetricRepository.findByProviderAndModelAndDimension(
                testProvider,
                "unused-model",
                128
        )).isEmpty();
    }

    @Test
    void shouldNotPropagateMetricUpdateFailure() {
        assertThatCode(() -> embeddingCacheMetricsService.recordHit(
                testProvider,
                MockEmbeddingClient.DEFAULT_MODEL,
                128
        )).doesNotThrowAnyException();
    }

    private EmbeddingCacheMetricItemResponse metricItem(String provider, String model, int dimension) {
        return embeddingCacheMetricsService.getMetrics().getItems().stream()
                .filter(item -> provider.equals(item.getProvider())
                        && model.equals(item.getModel())
                        && dimension == item.getDimension())
                .findFirst()
                .orElseThrow();
    }
}
