package com.tuoman.ai_task_orchestrator.embedding;

import org.junit.jupiter.api.Test;

import java.util.ArrayList;
import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class OpenAiCompatibleEmbeddingProviderTest {

    @Test
    void embedShouldCreateRequestAndParseSingleEmbeddingResponse() {
        FakeOpenAiEmbeddingHttpClient httpClient = new FakeOpenAiEmbeddingHttpClient(response(List.of(
                data(0, List.of(0.1, 0.2, 0.3))
        )));
        OpenAiCompatibleEmbeddingProvider provider = provider(httpClient, 3);

        EmbeddingRequest request = request("hello world");
        EmbeddingResponse response = provider.embed(request);

        assertThat(httpClient.requests).hasSize(1);
        assertThat(httpClient.requests.getFirst().getModel()).isEqualTo("text-embedding-test");
        assertThat(httpClient.requests.getFirst().getInput()).containsExactly("hello world");
        assertThat(response.getProvider()).isEqualTo("openai");
        assertThat(response.getModel()).isEqualTo("text-embedding-test");
        assertThat(response.getDimension()).isEqualTo(3);
        assertThat(response.getDistanceMetric()).isEqualTo("COSINE");
        assertThat(response.getVector()).containsExactly(0.1, 0.2, 0.3);
    }

    @Test
    void embedBatchShouldReturnVectorsInResponseIndexOrder() {
        FakeOpenAiEmbeddingHttpClient httpClient = new FakeOpenAiEmbeddingHttpClient(response(List.of(
                data(1, List.of(0.4, 0.5)),
                data(0, List.of(0.1, 0.2))
        )));
        OpenAiCompatibleEmbeddingProvider provider = provider(httpClient, 2);

        List<EmbeddingResponse> responses = provider.embedBatch(List.of(
                request("first"),
                request("second")
        ));

        assertThat(httpClient.requests.getFirst().getInput()).containsExactly("first", "second");
        assertThat(responses).hasSize(2);
        assertThat(responses.get(0).getVector()).containsExactly(0.1, 0.2);
        assertThat(responses.get(1).getVector()).containsExactly(0.4, 0.5);
        assertThat(responses).allSatisfy(response -> {
            assertThat(response.getProvider()).isEqualTo("openai");
            assertThat(response.getModel()).isEqualTo("text-embedding-test");
            assertThat(response.getDimension()).isEqualTo(2);
        });
    }

    @Test
    void embedBatchShouldReturnEmptyListWhenInputIsEmpty() {
        FakeOpenAiEmbeddingHttpClient httpClient = new FakeOpenAiEmbeddingHttpClient(response(List.of()));
        OpenAiCompatibleEmbeddingProvider provider = provider(httpClient, 2);

        assertThat(provider.embedBatch(List.of())).isEmpty();
        assertThat(httpClient.requests).isEmpty();
    }

    @Test
    void embedShouldFailWhenApiKeyIsBlank() {
        EmbeddingProperties.OpenAi properties = properties(2);
        properties.setApiKey(" ");
        OpenAiCompatibleEmbeddingProvider provider = new OpenAiCompatibleEmbeddingProvider(
                properties,
                new FakeOpenAiEmbeddingHttpClient(response(List.of(data(0, List.of(0.1, 0.2)))))
        );

        assertThatThrownBy(() -> provider.embed(request("hello")))
                .isInstanceOf(EmbeddingProviderException.class)
                .hasMessageContaining("api key");
    }

    @Test
    void embedShouldFailWhenResponseDataIsEmpty() {
        OpenAiCompatibleEmbeddingProvider provider = provider(
                new FakeOpenAiEmbeddingHttpClient(response(List.of())),
                2
        );

        assertThatThrownBy(() -> provider.embed(request("hello")))
                .isInstanceOf(EmbeddingProviderException.class)
                .hasMessageContaining("data");
    }

    @Test
    void embedShouldFailWhenVectorIsEmpty() {
        OpenAiCompatibleEmbeddingProvider provider = provider(
                new FakeOpenAiEmbeddingHttpClient(response(List.of(data(0, List.of())))),
                2
        );

        assertThatThrownBy(() -> provider.embed(request("hello")))
                .isInstanceOf(EmbeddingProviderException.class)
                .hasMessageContaining("vector");
    }

    @Test
    void embedBatchShouldFailWhenResponseCountDoesNotMatchInputCount() {
        OpenAiCompatibleEmbeddingProvider provider = provider(
                new FakeOpenAiEmbeddingHttpClient(response(List.of(data(0, List.of(0.1, 0.2))))),
                2
        );

        assertThatThrownBy(() -> provider.embedBatch(List.of(request("one"), request("two"))))
                .isInstanceOf(EmbeddingProviderException.class)
                .hasMessageContaining("size");
    }

    @Test
    void embedShouldFailWhenDimensionDoesNotMatchConfiguration() {
        OpenAiCompatibleEmbeddingProvider provider = provider(
                new FakeOpenAiEmbeddingHttpClient(response(List.of(data(0, List.of(0.1, 0.2, 0.3))))),
                2
        );

        assertThatThrownBy(() -> provider.embed(request("hello")))
                .isInstanceOf(EmbeddingProviderException.class)
                .hasMessageContaining("dimension");
    }

    @Test
    void embedShouldWrapClientException() {
        FakeOpenAiEmbeddingHttpClient httpClient = new FakeOpenAiEmbeddingHttpClient(null);
        httpClient.exception = new IllegalStateException("http 500");
        OpenAiCompatibleEmbeddingProvider provider = provider(httpClient, 2);

        assertThatThrownBy(() -> provider.embed(request("hello")))
                .isInstanceOf(EmbeddingProviderException.class)
                .hasMessageContaining("request failed")
                .hasCauseInstanceOf(IllegalStateException.class);
    }

    private OpenAiCompatibleEmbeddingProvider provider(
            FakeOpenAiEmbeddingHttpClient httpClient,
            int dimension
    ) {
        return new OpenAiCompatibleEmbeddingProvider(properties(dimension), httpClient);
    }

    private EmbeddingProperties.OpenAi properties(int dimension) {
        EmbeddingProperties.OpenAi properties = new EmbeddingProperties.OpenAi();
        properties.setApiKey("test-key");
        properties.setBaseUrl("https://example.test/v1");
        properties.setModel("text-embedding-test");
        properties.setDimension(dimension);
        properties.setTimeoutMs(1000);
        return properties;
    }

    private EmbeddingRequest request(String text) {
        EmbeddingRequest request = new EmbeddingRequest();
        request.setText(text);
        return request;
    }

    private OpenAiEmbeddingResponse response(List<OpenAiEmbeddingResponse.EmbeddingData> data) {
        OpenAiEmbeddingResponse response = new OpenAiEmbeddingResponse();
        response.setModel("text-embedding-test");
        response.setData(data);
        return response;
    }

    private OpenAiEmbeddingResponse.EmbeddingData data(Integer index, List<Double> vector) {
        OpenAiEmbeddingResponse.EmbeddingData data = new OpenAiEmbeddingResponse.EmbeddingData();
        data.setIndex(index);
        data.setEmbedding(vector);
        return data;
    }

    private static class FakeOpenAiEmbeddingHttpClient implements OpenAiEmbeddingHttpClient {

        private final OpenAiEmbeddingResponse response;

        private final List<OpenAiEmbeddingRequest> requests = new ArrayList<>();

        private RuntimeException exception;

        private FakeOpenAiEmbeddingHttpClient(OpenAiEmbeddingResponse response) {
            this.response = response;
        }

        @Override
        public OpenAiEmbeddingResponse createEmbeddings(
                OpenAiEmbeddingRequest request,
                EmbeddingProperties.OpenAi properties
        ) {
            requests.add(request);
            if (exception != null) {
                throw exception;
            }
            return response;
        }
    }
}
