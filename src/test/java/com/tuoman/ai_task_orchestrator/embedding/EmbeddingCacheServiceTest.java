package com.tuoman.ai_task_orchestrator.embedding;

import com.tuoman.ai_task_orchestrator.entity.EmbeddingCacheEntity;
import com.tuoman.ai_task_orchestrator.repository.EmbeddingCacheRepository;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;
import org.springframework.dao.DataIntegrityViolationException;

import java.util.List;
import java.util.Optional;
import java.util.concurrent.atomic.AtomicInteger;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.times;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class EmbeddingCacheServiceTest {

    private static final String CONTENT = "cache service chunk content";

    private static final String PROVIDER = MockEmbeddingClient.PROVIDER;

    private static final String MODEL = MockEmbeddingClient.DEFAULT_MODEL;

    private static final int DIMENSION = 2;

    @Mock
    private EmbeddingCacheRepository embeddingCacheRepository;

    @Mock
    private ChunkHashService chunkHashService;

    @Mock
    private EmbeddingCacheJsonCodec embeddingCacheJsonCodec;

    @Mock
    private EmbeddingProvider embeddingProvider;

    @Mock
    private EmbeddingCacheMetricsService embeddingCacheMetricsService;

    @InjectMocks
    private EmbeddingCacheService embeddingCacheService;

    @Test
    void shouldAcceptLocalWorkerRouteWithLocalOllamaRuntimeProvider() {
        String chunkHash = "hash-local-ollama";
        List<Double> vector = List.of(0.1, 0.2, 0.3, 0.4);
        int dimension = 4;
        String routeProvider = LocalEmbeddingWorkerProvider.PROVIDER;
        String model = "qwen3-embedding:0.6b";
        EmbeddingResponse response = new EmbeddingResponse(
                LocalEmbeddingWorkerProvider.RUNTIME_PROVIDER_OLLAMA,
                model,
                dimension,
                "COSINE",
                vector
        );

        when(chunkHashService.hash(CONTENT)).thenReturn(chunkHash);
        when(embeddingCacheRepository.findByChunkHashAndProviderAndModelAndDimension(
                chunkHash, routeProvider, model, dimension
        )).thenReturn(Optional.empty());
        when(embeddingProvider.embed(any(EmbeddingRequest.class))).thenReturn(response);
        when(embeddingCacheJsonCodec.serialize(vector)).thenReturn("[0.1,0.2,0.3,0.4]");
        when(embeddingCacheJsonCodec.deserialize("[0.1,0.2,0.3,0.4]")).thenReturn(vector);
        when(embeddingCacheRepository.saveAndFlush(any(EmbeddingCacheEntity.class))).thenAnswer(invocation -> {
            EmbeddingCacheEntity entity = invocation.getArgument(0);
            entity.setId(2L);
            return entity;
        });

        CachedEmbeddingResult result = embeddingCacheService.getOrCompute(CONTENT, routeProvider, model, dimension);

        assertThat(result.provider()).isEqualTo(routeProvider);
        assertThat(result.model()).isEqualTo(model);
        assertThat(result.dimension()).isEqualTo(dimension);
        assertThat(result.embedding()).isEqualTo(vector);
    }

    @Test
    void shouldRejectBlankRuntimeProvider() {
        when(chunkHashService.hash(CONTENT)).thenReturn("hash-blank-provider");
        when(embeddingCacheRepository.findByChunkHashAndProviderAndModelAndDimension(
                "hash-blank-provider", PROVIDER, MODEL, DIMENSION
        )).thenReturn(Optional.empty());
        when(embeddingProvider.embed(any(EmbeddingRequest.class)))
                .thenReturn(new EmbeddingResponse("  ", MODEL, DIMENSION, "COSINE", List.of(0.1, 0.2)));

        assertThatThrownBy(() -> embeddingCacheService.getOrCompute(CONTENT, PROVIDER, MODEL, DIMENSION))
                .isInstanceOf(EmbeddingProviderException.class)
                .hasMessageContaining("unexpected provider");
    }

    @Test
    void shouldCallProviderOnCacheMissAndPersistCache() {
        String chunkHash = "hash-miss";
        List<Double> vector = List.of(0.1, 0.2);
        EmbeddingResponse response = new EmbeddingResponse(PROVIDER, MODEL, DIMENSION, "COSINE", vector);

        when(chunkHashService.hash(CONTENT)).thenReturn(chunkHash);
        when(embeddingCacheRepository.findByChunkHashAndProviderAndModelAndDimension(
                chunkHash, PROVIDER, MODEL, DIMENSION
        )).thenReturn(Optional.empty());
        when(embeddingProvider.embed(any(EmbeddingRequest.class))).thenReturn(response);
        when(embeddingCacheJsonCodec.serialize(vector)).thenReturn("[0.1,0.2]");
        when(embeddingCacheJsonCodec.deserialize("[0.1,0.2]")).thenReturn(vector);
        when(embeddingCacheRepository.saveAndFlush(any(EmbeddingCacheEntity.class))).thenAnswer(invocation -> {
            EmbeddingCacheEntity entity = invocation.getArgument(0);
            entity.setId(1L);
            return entity;
        });

        CachedEmbeddingResult result = embeddingCacheService.getOrCompute(CONTENT, PROVIDER, MODEL, DIMENSION);

        assertThat(result.cacheHit()).isFalse();
        assertThat(result.chunkHash()).isEqualTo(chunkHash);
        assertThat(result.provider()).isEqualTo(PROVIDER);
        assertThat(result.model()).isEqualTo(MODEL);
        assertThat(result.dimension()).isEqualTo(DIMENSION);
        assertThat(result.embedding()).isEqualTo(vector);
        verify(embeddingProvider, times(1)).embed(any(EmbeddingRequest.class));
        verify(embeddingCacheRepository).saveAndFlush(any(EmbeddingCacheEntity.class));
    }

    @Test
    void shouldNotCallProviderOnCacheHit() {
        String chunkHash = "hash-hit";
        List<Double> vector = List.of(0.3, 0.4);
        EmbeddingCacheEntity entity = new EmbeddingCacheEntity();
        entity.setChunkHash(chunkHash);
        entity.setProvider(PROVIDER);
        entity.setModel(MODEL);
        entity.setDimension(DIMENSION);
        entity.setEmbeddingJson("[0.3,0.4]");

        when(chunkHashService.hash(CONTENT)).thenReturn(chunkHash);
        when(embeddingCacheRepository.findByChunkHashAndProviderAndModelAndDimension(
                chunkHash, PROVIDER, MODEL, DIMENSION
        )).thenReturn(Optional.of(entity));
        when(embeddingCacheJsonCodec.deserialize(entity.getEmbeddingJson())).thenReturn(vector);

        CachedEmbeddingResult result = embeddingCacheService.getOrCompute(CONTENT, PROVIDER, MODEL, DIMENSION);

        assertThat(result.cacheHit()).isTrue();
        assertThat(result.embedding()).isEqualTo(vector);
        verify(embeddingProvider, never()).embed(any(EmbeddingRequest.class));
        verify(embeddingCacheRepository, never()).saveAndFlush(any());
    }

    @Test
    void shouldUseDifferentCacheEntriesForDifferentModels() {
        String chunkHash = "hash-shared";
        when(chunkHashService.hash(CONTENT)).thenReturn(chunkHash);
        when(embeddingCacheRepository.findByChunkHashAndProviderAndModelAndDimension(
                chunkHash, PROVIDER, "model-a", DIMENSION
        )).thenReturn(Optional.empty());
        when(embeddingProvider.embed(any(EmbeddingRequest.class)))
                .thenReturn(new EmbeddingResponse(PROVIDER, "model-a", DIMENSION, "COSINE", List.of(0.1, 0.2)));
        when(embeddingCacheJsonCodec.serialize(List.of(0.1, 0.2))).thenReturn("[0.1,0.2]");
        when(embeddingCacheJsonCodec.deserialize("[0.1,0.2]")).thenReturn(List.of(0.1, 0.2));
        when(embeddingCacheRepository.saveAndFlush(any(EmbeddingCacheEntity.class))).thenAnswer(invocation -> invocation.getArgument(0));

        embeddingCacheService.getOrCompute(CONTENT, PROVIDER, "model-a", DIMENSION, embeddingProvider);

        verify(embeddingCacheRepository, never()).findByChunkHashAndProviderAndModelAndDimension(
                chunkHash, PROVIDER, "model-b", DIMENSION
        );
    }

    @Test
    void shouldNotPersistCacheWhenProviderThrows() {
        String chunkHash = "hash-failure";
        when(chunkHashService.hash(CONTENT)).thenReturn(chunkHash);
        when(embeddingCacheRepository.findByChunkHashAndProviderAndModelAndDimension(
                chunkHash, PROVIDER, MODEL, DIMENSION
        )).thenReturn(Optional.empty());
        when(embeddingProvider.embed(any(EmbeddingRequest.class)))
                .thenThrow(new EmbeddingProviderException("provider failed"));

        assertThatThrownBy(() -> embeddingCacheService.getOrCompute(CONTENT, PROVIDER, MODEL, DIMENSION))
                .isInstanceOf(EmbeddingProviderException.class);

        verify(embeddingCacheRepository, never()).saveAndFlush(any());
    }

    @Test
    void shouldReloadCacheAfterUniqueKeyConflict() {
        String chunkHash = "hash-conflict";
        List<Double> vector = List.of(0.5, 0.6);
        EmbeddingResponse response = new EmbeddingResponse(PROVIDER, MODEL, DIMENSION, "COSINE", vector);
        EmbeddingCacheEntity existing = new EmbeddingCacheEntity();
        existing.setChunkHash(chunkHash);
        existing.setProvider(PROVIDER);
        existing.setModel(MODEL);
        existing.setDimension(DIMENSION);
        existing.setEmbeddingJson("[0.5,0.6]");

        when(chunkHashService.hash(CONTENT)).thenReturn(chunkHash);
        when(embeddingCacheRepository.findByChunkHashAndProviderAndModelAndDimension(
                chunkHash, PROVIDER, MODEL, DIMENSION
        )).thenReturn(Optional.empty(), Optional.of(existing));
        when(embeddingProvider.embed(any(EmbeddingRequest.class))).thenReturn(response);
        when(embeddingCacheJsonCodec.serialize(vector)).thenReturn("[0.5,0.6]");
        when(embeddingCacheRepository.saveAndFlush(any(EmbeddingCacheEntity.class)))
                .thenThrow(new DataIntegrityViolationException("duplicate key"));
        when(embeddingCacheJsonCodec.deserialize(existing.getEmbeddingJson())).thenReturn(vector);

        CachedEmbeddingResult result = embeddingCacheService.getOrCompute(CONTENT, PROVIDER, MODEL, DIMENSION);

        assertThat(result.cacheHit()).isTrue();
        assertThat(result.embedding()).isEqualTo(vector);
    }
}
