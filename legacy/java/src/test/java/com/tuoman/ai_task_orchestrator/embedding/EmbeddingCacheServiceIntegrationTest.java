package com.tuoman.ai_task_orchestrator.embedding;

import com.tuoman.ai_task_orchestrator.repository.EmbeddingCacheRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.annotation.Transactional;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest
@Transactional
class EmbeddingCacheServiceIntegrationTest {

    private static final String CONTENT = "integration cache chunk content";

    @Autowired
    private EmbeddingCacheService embeddingCacheService;

    @Autowired
    private EmbeddingCacheRepository embeddingCacheRepository;

    @Autowired
    private ChunkHashService chunkHashService;

    private String testProviderSuffix;

    @BeforeEach
    void setUpUniqueProviderSuffix() {
        testProviderSuffix = "cache-it-" + System.nanoTime();
    }

    @Test
    void shouldHitCacheForSameProviderModelAndDimension() {
        MockEmbeddingClient provider = new MockEmbeddingClient();

        CachedEmbeddingResult first = embeddingCacheService.getOrCompute(
                CONTENT,
                provider.provider(),
                provider.model(),
                provider.dimension(),
                provider
        );
        CachedEmbeddingResult second = embeddingCacheService.getOrCompute(
                CONTENT,
                provider.provider(),
                provider.model(),
                provider.dimension(),
                provider
        );

        assertThat(first.cacheHit()).isFalse();
        assertThat(second.cacheHit()).isTrue();
        assertThat(second.embedding()).isEqualTo(first.embedding());
        assertThat(cacheEntryExists(provider.provider(), provider.model(), provider.dimension())).isTrue();
    }

    @Test
    void shouldMissCacheForDifferentModel() {
        MockEmbeddingClient provider = new MockEmbeddingClient();

        embeddingCacheService.getOrCompute(
                CONTENT,
                provider.provider(),
                "model-a",
                provider.dimension(),
                fixedResponseProvider(provider, "model-a", provider.dimension())
        );

        CachedEmbeddingResult differentModel = embeddingCacheService.getOrCompute(
                CONTENT,
                provider.provider(),
                "model-b",
                provider.dimension(),
                fixedResponseProvider(provider, "model-b", provider.dimension())
        );

        assertThat(differentModel.cacheHit()).isFalse();
        assertThat(cacheEntryExists(provider.provider(), "model-a", provider.dimension())).isTrue();
        assertThat(cacheEntryExists(provider.provider(), "model-b", provider.dimension())).isTrue();
    }

    @Test
    void shouldMissCacheForDifferentDimension() {
        MockEmbeddingClient provider = new MockEmbeddingClient();
        List<Double> vector128 = provider.embed(embeddingRequest(provider, CONTENT)).getVector();
        List<Double> vector64 = vector128.subList(0, 64);

        embeddingCacheService.getOrCompute(
                CONTENT,
                provider.provider(),
                provider.model(),
                128,
                fixedVectorProvider(provider, 128, vector128)
        );

        CachedEmbeddingResult differentDimension = embeddingCacheService.getOrCompute(
                CONTENT,
                provider.provider(),
                provider.model(),
                64,
                fixedVectorProvider(provider, 64, vector64)
        );

        assertThat(differentDimension.cacheHit()).isFalse();
        assertThat(cacheEntryExists(provider.provider(), provider.model(), 128)).isTrue();
        assertThat(cacheEntryExists(provider.provider(), provider.model(), 64)).isTrue();
    }

    @Test
    void shouldMissCacheForDifferentProvider() {
        MockEmbeddingClient provider = new MockEmbeddingClient();
        List<Double> vector = provider.embed(embeddingRequest(provider, CONTENT)).getVector();

        String providerA = scopedProvider("provider-a");
        String providerB = scopedProvider("provider-b");

        embeddingCacheService.getOrCompute(
                CONTENT,
                providerA,
                provider.model(),
                provider.dimension(),
                namedProvider(providerA, provider.model(), provider.dimension(), vector)
        );

        CachedEmbeddingResult differentProvider = embeddingCacheService.getOrCompute(
                CONTENT,
                providerB,
                provider.model(),
                provider.dimension(),
                namedProvider(providerB, provider.model(), provider.dimension(), vector)
        );

        assertThat(differentProvider.cacheHit()).isFalse();
        assertThat(cacheEntryExists(providerA, provider.model(), provider.dimension())).isTrue();
        assertThat(cacheEntryExists(providerB, provider.model(), provider.dimension())).isTrue();
    }

    private String scopedProvider(String base) {
        return base + "-" + testProviderSuffix;
    }

    private boolean cacheEntryExists(String provider, String model, int dimension) {
        String chunkHash = chunkHashService.hash(CONTENT);
        return embeddingCacheRepository
                .findByChunkHashAndProviderAndModelAndDimension(chunkHash, provider, model, dimension)
                .isPresent();
    }

    private EmbeddingProvider fixedResponseProvider(
            MockEmbeddingClient delegate,
            String model,
            int dimension
    ) {
        return new EmbeddingProvider() {
            @Override
            public EmbeddingResponse embed(EmbeddingRequest request) {
                return new EmbeddingResponse(
                        delegate.provider(),
                        model,
                        dimension,
                        MockEmbeddingClient.DISTANCE_METRIC,
                        delegate.embed(request).getVector()
                );
            }

            @Override
            public String provider() {
                return delegate.provider();
            }

            @Override
            public String model() {
                return model;
            }

            @Override
            public int dimension() {
                return dimension;
            }
        };
    }

    private EmbeddingProvider fixedVectorProvider(
            MockEmbeddingClient delegate,
            int dimension,
            List<Double> vector
    ) {
        return new EmbeddingProvider() {
            @Override
            public EmbeddingResponse embed(EmbeddingRequest request) {
                return new EmbeddingResponse(
                        delegate.provider(),
                        delegate.model(),
                        dimension,
                        MockEmbeddingClient.DISTANCE_METRIC,
                        vector
                );
            }

            @Override
            public String provider() {
                return delegate.provider();
            }

            @Override
            public String model() {
                return delegate.model();
            }

            @Override
            public int dimension() {
                return dimension;
            }
        };
    }

    private EmbeddingProvider namedProvider(
            String providerName,
            String model,
            int dimension,
            List<Double> vector
    ) {
        return new EmbeddingProvider() {
            @Override
            public EmbeddingResponse embed(EmbeddingRequest request) {
                return new EmbeddingResponse(
                        providerName,
                        model,
                        dimension,
                        MockEmbeddingClient.DISTANCE_METRIC,
                        vector
                );
            }

            @Override
            public String provider() {
                return providerName;
            }

            @Override
            public String model() {
                return model;
            }

            @Override
            public int dimension() {
                return dimension;
            }
        };
    }

    private EmbeddingRequest embeddingRequest(MockEmbeddingClient provider, String text) {
        EmbeddingRequest request = new EmbeddingRequest();
        request.setText(text);
        request.setModel(provider.model());
        return request;
    }
}
