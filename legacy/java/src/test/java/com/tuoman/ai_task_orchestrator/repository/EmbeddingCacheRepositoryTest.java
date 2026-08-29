package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.EmbeddingCacheEntity;
import com.tuoman.ai_task_orchestrator.embedding.ChunkHashService;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingCacheJsonCodec;
import com.tuoman.ai_task_orchestrator.embedding.MockEmbeddingClient;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.dao.DataIntegrityViolationException;
import org.springframework.transaction.annotation.Transactional;

import java.util.List;
import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

@SpringBootTest
@Transactional
class EmbeddingCacheRepositoryTest {

    @Autowired
    private EmbeddingCacheRepository embeddingCacheRepository;

    @Autowired
    private ChunkHashService chunkHashService;

    @Autowired
    private EmbeddingCacheJsonCodec embeddingCacheJsonCodec;

    @Test
    void shouldFindByChunkHashProviderModelAndDimension() {
        String chunkHash = chunkHashService.hash("repository lookup chunk");
        saveCache(chunkHash, MockEmbeddingClient.PROVIDER, MockEmbeddingClient.DEFAULT_MODEL, MockEmbeddingClient.DIMENSION);

        Optional<EmbeddingCacheEntity> found = embeddingCacheRepository.findByChunkHashAndProviderAndModelAndDimension(
                chunkHash,
                MockEmbeddingClient.PROVIDER,
                MockEmbeddingClient.DEFAULT_MODEL,
                MockEmbeddingClient.DIMENSION
        );

        assertThat(found).isPresent();
        assertThat(found.get().getEmbeddingJson()).contains("0.");
    }

    @Test
    void shouldRejectDuplicateCacheKey() {
        String chunkHash = chunkHashService.hash("duplicate cache key chunk");
        saveCache(chunkHash, MockEmbeddingClient.PROVIDER, MockEmbeddingClient.DEFAULT_MODEL, MockEmbeddingClient.DIMENSION);

        EmbeddingCacheEntity duplicate = new EmbeddingCacheEntity();
        duplicate.setChunkHash(chunkHash);
        duplicate.setProvider(MockEmbeddingClient.PROVIDER);
        duplicate.setModel(MockEmbeddingClient.DEFAULT_MODEL);
        duplicate.setDimension(MockEmbeddingClient.DIMENSION);
        duplicate.setEmbeddingJson(embeddingCacheJsonCodec.serialize(List.of(0.1, 0.2)));

        assertThatThrownBy(() -> embeddingCacheRepository.saveAndFlush(duplicate))
                .isInstanceOf(DataIntegrityViolationException.class);
    }

    @Test
    void shouldAllowSameChunkHashWithDifferentModel() {
        String chunkHash = chunkHashService.hash("shared chunk different model");
        saveCache(chunkHash, MockEmbeddingClient.PROVIDER, "model-a", MockEmbeddingClient.DIMENSION);
        saveCache(chunkHash, MockEmbeddingClient.PROVIDER, "model-b", MockEmbeddingClient.DIMENSION);

        assertThat(embeddingCacheRepository.findByChunkHashAndProviderAndModelAndDimension(
                chunkHash, MockEmbeddingClient.PROVIDER, "model-a", MockEmbeddingClient.DIMENSION
        )).isPresent();
        assertThat(embeddingCacheRepository.findByChunkHashAndProviderAndModelAndDimension(
                chunkHash, MockEmbeddingClient.PROVIDER, "model-b", MockEmbeddingClient.DIMENSION
        )).isPresent();
    }

    @Test
    void shouldAllowSameChunkHashWithDifferentDimension() {
        String chunkHash = chunkHashService.hash("shared chunk different dimension");
        saveCache(chunkHash, MockEmbeddingClient.PROVIDER, MockEmbeddingClient.DEFAULT_MODEL, 128);
        saveCache(chunkHash, MockEmbeddingClient.PROVIDER, MockEmbeddingClient.DEFAULT_MODEL, 256);

        assertThat(embeddingCacheRepository.findByChunkHashAndProviderAndModelAndDimension(
                chunkHash, MockEmbeddingClient.PROVIDER, MockEmbeddingClient.DEFAULT_MODEL, 128
        )).isPresent();
        assertThat(embeddingCacheRepository.findByChunkHashAndProviderAndModelAndDimension(
                chunkHash, MockEmbeddingClient.PROVIDER, MockEmbeddingClient.DEFAULT_MODEL, 256
        )).isPresent();
    }

    @Test
    void shouldAllowSameChunkHashWithDifferentProvider() {
        String chunkHash = chunkHashService.hash("shared chunk different provider");
        saveCache(chunkHash, "provider-a", MockEmbeddingClient.DEFAULT_MODEL, MockEmbeddingClient.DIMENSION);
        saveCache(chunkHash, "provider-b", MockEmbeddingClient.DEFAULT_MODEL, MockEmbeddingClient.DIMENSION);

        assertThat(embeddingCacheRepository.findByChunkHashAndProviderAndModelAndDimension(
                chunkHash, "provider-a", MockEmbeddingClient.DEFAULT_MODEL, MockEmbeddingClient.DIMENSION
        )).isPresent();
        assertThat(embeddingCacheRepository.findByChunkHashAndProviderAndModelAndDimension(
                chunkHash, "provider-b", MockEmbeddingClient.DEFAULT_MODEL, MockEmbeddingClient.DIMENSION
        )).isPresent();
    }

    @Test
    void shouldPersistAndReadEmbeddingJson() {
        String chunkHash = chunkHashService.hash("embedding json persistence");
        List<Double> vector = List.of(0.1, 0.2, 0.3);
        EmbeddingCacheEntity entity = new EmbeddingCacheEntity();
        entity.setChunkHash(chunkHash);
        entity.setProvider(MockEmbeddingClient.PROVIDER);
        entity.setModel(MockEmbeddingClient.DEFAULT_MODEL);
        entity.setDimension(vector.size());
        entity.setEmbeddingJson(embeddingCacheJsonCodec.serialize(vector));
        embeddingCacheRepository.saveAndFlush(entity);

        EmbeddingCacheEntity loaded = embeddingCacheRepository.findById(entity.getId()).orElseThrow();
        assertThat(embeddingCacheJsonCodec.deserialize(loaded.getEmbeddingJson())).containsExactly(0.1, 0.2, 0.3);
    }

    private void saveCache(String chunkHash, String provider, String model, int dimension) {
        EmbeddingCacheEntity entity = new EmbeddingCacheEntity();
        entity.setChunkHash(chunkHash);
        entity.setProvider(provider);
        entity.setModel(model);
        entity.setDimension(dimension);
        entity.setEmbeddingJson(embeddingCacheJsonCodec.serialize(sampleVector(dimension)));
        embeddingCacheRepository.saveAndFlush(entity);
    }

    private List<Double> sampleVector(int dimension) {
        return java.util.stream.IntStream.range(0, dimension)
                .mapToObj(index -> (index + 1) / (double) dimension)
                .toList();
    }
}
