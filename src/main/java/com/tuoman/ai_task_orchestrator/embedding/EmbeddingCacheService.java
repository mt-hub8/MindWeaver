package com.tuoman.ai_task_orchestrator.embedding;

import com.tuoman.ai_task_orchestrator.entity.EmbeddingCacheEntity;
import com.tuoman.ai_task_orchestrator.repository.EmbeddingCacheRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.dao.DataIntegrityViolationException;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.List;
import java.util.Optional;

@Service
@RequiredArgsConstructor
public class EmbeddingCacheService {

    private static final String DEFAULT_DISTANCE_METRIC = "COSINE";

    private final EmbeddingCacheRepository embeddingCacheRepository;

    private final ChunkHashService chunkHashService;

    private final EmbeddingCacheJsonCodec embeddingCacheJsonCodec;

    private final EmbeddingProvider embeddingProvider;

    private final EmbeddingCacheMetricsService embeddingCacheMetricsService;

    @Transactional
    public CachedEmbeddingResult getOrCompute(
            String chunkContent,
            String provider,
            String model,
            int dimension
    ) {
        return getOrCompute(chunkContent, provider, model, dimension, embeddingProvider);
    }

    @Transactional
    public CachedEmbeddingResult getOrCompute(
            String chunkContent,
            String provider,
            String model,
            int dimension,
            EmbeddingProvider providerForCompute
    ) {
        validateCacheKey(provider, model, dimension);
        if (providerForCompute == null) {
            throw new IllegalArgumentException("providerForCompute must not be null");
        }

        String chunkHash = chunkHashService.hash(chunkContent);
        Optional<EmbeddingCacheEntity> cached = embeddingCacheRepository
                .findByChunkHashAndProviderAndModelAndDimension(chunkHash, provider, model, dimension);
        if (cached.isPresent()) {
            embeddingCacheMetricsService.recordHit(provider, model, dimension);
            return toCachedResult(cached.get(), chunkHash, true);
        }

        embeddingCacheMetricsService.recordMiss(provider, model, dimension);

        EmbeddingRequest request = new EmbeddingRequest();
        request.setText(chunkContent);
        request.setModel(model);

        EmbeddingResponse response = providerForCompute.embed(request);

        validateEmbeddingResponse(response, provider, model, dimension);

        EmbeddingCacheEntity entity = new EmbeddingCacheEntity();
        entity.setChunkHash(chunkHash);
        entity.setProvider(provider);
        entity.setModel(model);
        entity.setDimension(dimension);
        entity.setEmbeddingJson(embeddingCacheJsonCodec.serialize(response.getVector()));

        try {
            EmbeddingCacheEntity saved = embeddingCacheRepository.saveAndFlush(entity);
            embeddingCacheMetricsService.recordWrite(provider, model, dimension);
            return toCachedResult(saved, chunkHash, false, response.getDistanceMetric());
        } catch (DataIntegrityViolationException exception) {
            embeddingCacheMetricsService.recordConflict(provider, model, dimension);
            return embeddingCacheRepository
                    .findByChunkHashAndProviderAndModelAndDimension(chunkHash, provider, model, dimension)
                    .map(existing -> toCachedResult(existing, chunkHash, true))
                    .orElseThrow(() -> new IllegalStateException(
                            "embedding cache unique key conflict but record not found",
                            exception
                    ));
        }
    }

    private CachedEmbeddingResult toCachedResult(
            EmbeddingCacheEntity entity,
            String chunkHash,
            boolean cacheHit
    ) {
        return toCachedResult(entity, chunkHash, cacheHit, DEFAULT_DISTANCE_METRIC);
    }

    private CachedEmbeddingResult toCachedResult(
            EmbeddingCacheEntity entity,
            String chunkHash,
            boolean cacheHit,
            String distanceMetric
    ) {
        List<Double> vector = embeddingCacheJsonCodec.deserialize(entity.getEmbeddingJson());
        if (!entity.getDimension().equals(vector.size())) {
            throw new EmbeddingProviderException("cached embedding dimension mismatch");
        }
        return new CachedEmbeddingResult(
                vector,
                entity.getProvider(),
                entity.getModel(),
                entity.getDimension(),
                distanceMetric == null || distanceMetric.isBlank() ? DEFAULT_DISTANCE_METRIC : distanceMetric,
                chunkHash,
                cacheHit
        );
    }

    private void validateCacheKey(String provider, String model, int dimension) {
        if (provider == null || provider.isBlank()) {
            throw new IllegalArgumentException("provider must not be blank");
        }
        if (model == null || model.isBlank()) {
            throw new IllegalArgumentException("model must not be blank");
        }
        if (dimension <= 0) {
            throw new IllegalArgumentException("dimension must be greater than 0");
        }
    }

    private void validateEmbeddingResponse(
            EmbeddingResponse response,
            String expectedProvider,
            String expectedModel,
            int expectedDimension
    ) {
        if (response == null) {
            throw new EmbeddingProviderException("embedding provider returned null response");
        }
        if (response.getVector() == null || response.getVector().isEmpty()) {
            throw new EmbeddingProviderException("embedding provider returned empty vector");
        }
        if (!LocalEmbeddingWorkerProvider.isProviderCompatible(expectedProvider, response.getProvider())) {
            throw new EmbeddingProviderException("embedding provider returned unexpected provider");
        }
        if (response.getModel() == null || !response.getModel().equals(expectedModel)) {
            throw new EmbeddingProviderException("embedding provider returned unexpected model");
        }
        if (response.getDimension() == null || !response.getDimension().equals(expectedDimension)) {
            throw new EmbeddingProviderException("embedding provider returned unexpected dimension");
        }
        if (response.getVector().size() != expectedDimension) {
            throw new EmbeddingProviderException("embedding vector size does not match expected dimension");
        }
    }
}
