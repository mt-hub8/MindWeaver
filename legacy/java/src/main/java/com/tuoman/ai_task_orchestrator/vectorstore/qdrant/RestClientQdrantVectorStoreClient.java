package com.tuoman.ai_task_orchestrator.vectorstore.qdrant;

import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreProperties;
import org.springframework.http.HttpHeaders;
import org.springframework.http.client.JdkClientHttpRequestFactory;
import org.springframework.stereotype.Component;
import org.springframework.web.client.RestClient;

import java.time.Duration;

@Component
public class RestClientQdrantVectorStoreClient implements QdrantVectorStoreClient {

    private final RestClient.Builder restClientBuilder;

    public RestClientQdrantVectorStoreClient(RestClient.Builder restClientBuilder) {
        this.restClientBuilder = restClientBuilder;
    }

    @Override
    public void createCollectionIfNeeded(
            VectorStoreProperties.Qdrant properties,
            QdrantCreateCollectionRequest request
    ) {
        try {
            restClient(properties).put()
                    .uri("/collections/{collectionName}", properties.getCollectionName())
                    .body(request)
                    .retrieve()
                    .toBodilessEntity();
        } catch (Exception exception) {
            throw new QdrantVectorStoreException("Qdrant collection initialization failed", exception);
        }
    }

    @Override
    public void upsertPoints(
            VectorStoreProperties.Qdrant properties,
            QdrantUpsertPointsRequest request
    ) {
        try {
            restClient(properties).put()
                    .uri("/collections/{collectionName}/points?wait=true", properties.getCollectionName())
                    .body(request)
                    .retrieve()
                    .toBodilessEntity();
        } catch (Exception exception) {
            throw new QdrantVectorStoreException("Qdrant point upsert failed", exception);
        }
    }

    @Override
    public QdrantSearchResponse searchPoints(
            VectorStoreProperties.Qdrant properties,
            QdrantSearchRequest request
    ) {
        try {
            return restClient(properties).post()
                    .uri("/collections/{collectionName}/points/search", properties.getCollectionName())
                    .body(request)
                    .retrieve()
                    .body(QdrantSearchResponse.class);
        } catch (Exception exception) {
            throw new QdrantVectorStoreException("Qdrant point search failed", exception);
        }
    }

    @Override
    public void deletePoints(
            VectorStoreProperties.Qdrant properties,
            QdrantDeletePointsRequest request
    ) {
        try {
            restClient(properties).post()
                    .uri("/collections/{collectionName}/points/delete", properties.getCollectionName())
                    .body(request)
                    .retrieve()
                    .toBodilessEntity();
        } catch (Exception exception) {
            throw new QdrantVectorStoreException("Qdrant point delete failed", exception);
        }
    }

    private RestClient restClient(VectorStoreProperties.Qdrant properties) {
        RestClient.Builder builder = restClientBuilder
                .baseUrl(normalizeBaseUrl(properties.getBaseUrl()))
                .requestFactory(new JdkClientHttpRequestFactory(
                        java.net.http.HttpClient.newBuilder()
                                .connectTimeout(Duration.ofMillis(properties.getTimeoutMs()))
                                .build()
                ));

        if (properties.getApiKey() != null && !properties.getApiKey().isBlank()) {
            builder.defaultHeader(HttpHeaders.AUTHORIZATION, "Bearer " + properties.getApiKey());
            builder.defaultHeader("api-key", properties.getApiKey());
        }

        return builder.build();
    }

    private String normalizeBaseUrl(String baseUrl) {
        if (baseUrl == null || baseUrl.isBlank()) {
            return "http://127.0.0.1:6333";
        }
        return baseUrl.endsWith("/") ? baseUrl.substring(0, baseUrl.length() - 1) : baseUrl;
    }
}
