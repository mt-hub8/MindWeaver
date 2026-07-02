package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.EmbeddingCacheMetricEntity;
import com.tuoman.ai_task_orchestrator.embedding.MockEmbeddingClient;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.dao.DataIntegrityViolationException;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

@SpringBootTest
@Transactional
class EmbeddingCacheMetricRepositoryTest {

    @Autowired
    private EmbeddingCacheMetricRepository embeddingCacheMetricRepository;

    private String testProvider;

    @BeforeEach
    void setUpUniqueProvider() {
        testProvider = "metric-repo-" + System.nanoTime();
    }

    @Test
    void shouldCreateMetricRow() {
        EmbeddingCacheMetricEntity entity = saveMetric(
                testProvider,
                MockEmbeddingClient.DEFAULT_MODEL,
                MockEmbeddingClient.DIMENSION
        );

        assertThat(entity.getId()).isNotNull();
        assertThat(embeddingCacheMetricRepository.findByProviderAndModelAndDimension(
                testProvider,
                MockEmbeddingClient.DEFAULT_MODEL,
                MockEmbeddingClient.DIMENSION
        )).isPresent();
    }

    @Test
    void shouldIncrementHitCountAtomically() {
        saveMetric(testProvider, MockEmbeddingClient.DEFAULT_MODEL, MockEmbeddingClient.DIMENSION);

        int updated = embeddingCacheMetricRepository.incrementHit(
                testProvider,
                MockEmbeddingClient.DEFAULT_MODEL,
                MockEmbeddingClient.DIMENSION,
                LocalDateTime.now()
        );

        EmbeddingCacheMetricEntity loaded = embeddingCacheMetricRepository.findByProviderAndModelAndDimension(
                testProvider,
                MockEmbeddingClient.DEFAULT_MODEL,
                MockEmbeddingClient.DIMENSION
        ).orElseThrow();

        assertThat(updated).isEqualTo(1);
        assertThat(loaded.getHitCount()).isEqualTo(1L);
        assertThat(loaded.getSavedProviderCallCount()).isEqualTo(1L);
    }

    @Test
    void shouldIncrementMissCountAtomically() {
        saveMetric(testProvider, MockEmbeddingClient.DEFAULT_MODEL, MockEmbeddingClient.DIMENSION);

        embeddingCacheMetricRepository.incrementMiss(
                testProvider,
                MockEmbeddingClient.DEFAULT_MODEL,
                MockEmbeddingClient.DIMENSION,
                LocalDateTime.now()
        );

        EmbeddingCacheMetricEntity loaded = embeddingCacheMetricRepository.findByProviderAndModelAndDimension(
                testProvider,
                MockEmbeddingClient.DEFAULT_MODEL,
                MockEmbeddingClient.DIMENSION
        ).orElseThrow();

        assertThat(loaded.getMissCount()).isEqualTo(1L);
        assertThat(loaded.getProviderCallCount()).isEqualTo(1L);
    }

    @Test
    void shouldIncrementWriteCountAtomically() {
        saveMetric(testProvider, MockEmbeddingClient.DEFAULT_MODEL, MockEmbeddingClient.DIMENSION);

        embeddingCacheMetricRepository.incrementWrite(
                testProvider,
                MockEmbeddingClient.DEFAULT_MODEL,
                MockEmbeddingClient.DIMENSION,
                LocalDateTime.now()
        );

        assertThat(embeddingCacheMetricRepository.findByProviderAndModelAndDimension(
                testProvider,
                MockEmbeddingClient.DEFAULT_MODEL,
                MockEmbeddingClient.DIMENSION
        ).orElseThrow().getWriteCount()).isEqualTo(1L);
    }

    @Test
    void shouldIncrementConflictCountAtomically() {
        saveMetric(testProvider, MockEmbeddingClient.DEFAULT_MODEL, MockEmbeddingClient.DIMENSION);

        embeddingCacheMetricRepository.incrementConflict(
                testProvider,
                MockEmbeddingClient.DEFAULT_MODEL,
                MockEmbeddingClient.DIMENSION,
                LocalDateTime.now()
        );

        assertThat(embeddingCacheMetricRepository.findByProviderAndModelAndDimension(
                testProvider,
                MockEmbeddingClient.DEFAULT_MODEL,
                MockEmbeddingClient.DIMENSION
        ).orElseThrow().getConflictCount()).isEqualTo(1L);
    }

    @Test
    void shouldRejectDuplicateEmbeddingSpace() {
        saveMetric(testProvider, MockEmbeddingClient.DEFAULT_MODEL, MockEmbeddingClient.DIMENSION);

        EmbeddingCacheMetricEntity duplicate = new EmbeddingCacheMetricEntity();
        duplicate.setProvider(testProvider);
        duplicate.setModel(MockEmbeddingClient.DEFAULT_MODEL);
        duplicate.setDimension(MockEmbeddingClient.DIMENSION);

        assertThatThrownBy(() -> embeddingCacheMetricRepository.saveAndFlush(duplicate))
                .isInstanceOf(DataIntegrityViolationException.class);
    }

    @Test
    void shouldTrackDifferentEmbeddingSpacesSeparately() {
        String providerA = testProvider + "-a";
        String providerB = testProvider + "-b";
        saveMetric(providerA, "model-a", 128);
        saveMetric(providerB, "model-b", 256);

        embeddingCacheMetricRepository.incrementHit(providerA, "model-a", 128, LocalDateTime.now());
        embeddingCacheMetricRepository.incrementMiss(providerB, "model-b", 256, LocalDateTime.now());

        assertThat(embeddingCacheMetricRepository.findByProviderAndModelAndDimension(providerA, "model-a", 128)
                .orElseThrow().getHitCount()).isEqualTo(1L);
        assertThat(embeddingCacheMetricRepository.findByProviderAndModelAndDimension(providerB, "model-b", 256)
                .orElseThrow().getMissCount()).isEqualTo(1L);
    }

    private EmbeddingCacheMetricEntity saveMetric(String provider, String model, int dimension) {
        EmbeddingCacheMetricEntity entity = new EmbeddingCacheMetricEntity();
        entity.setProvider(provider);
        entity.setModel(model);
        entity.setDimension(dimension);
        return embeddingCacheMetricRepository.saveAndFlush(entity);
    }
}
