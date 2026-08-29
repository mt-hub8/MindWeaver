package com.tuoman.ai_task_orchestrator.embedding;

import org.junit.jupiter.api.Test;

import java.util.ArrayList;
import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class LocalEmbeddingWorkerProviderTest {

    @Test
    void runtimeProviderShouldExposeOllamaRouteForMetadata() {
        LocalEmbeddingWorkerProvider provider = provider(new FakeLocalEmbeddingWorkerClient(
                responseWithProvider("local-ollama", List.of(data(0, List.of(0.1, 0.2, 0.3))))
        ), 3);

        assertThat(provider.provider()).isEqualTo("local-worker");
        assertThat(provider.runtimeProvider()).isEqualTo("local-ollama");
        assertThat(provider.model()).isEqualTo("sentence-transformers/test-model");
        assertThat(provider.dimension()).isEqualTo(3);
    }

    @Test
    void embedShouldAcceptLocalOllamaRuntimeProvider() {
        FakeLocalEmbeddingWorkerClient client = new FakeLocalEmbeddingWorkerClient(
                responseWithProvider("local-ollama", List.of(data(0, List.of(0.1, 0.2, 0.3))))
        );
        LocalEmbeddingWorkerProvider provider = provider(client, 3);

        EmbeddingResponse response = provider.embed(request("hello ollama"));

        assertThat(response.getProvider()).isEqualTo("local-ollama");
        assertThat(response.getModel()).isEqualTo("sentence-transformers/test-model");
        assertThat(response.getDimension()).isEqualTo(3);
    }

    @Test
    void embedShouldAcceptLocalPythonRuntimeProvider() {
        FakeLocalEmbeddingWorkerClient client = new FakeLocalEmbeddingWorkerClient(
                responseWithProvider("local-python", List.of(data(0, List.of(0.1, 0.2))))
        );
        LocalEmbeddingWorkerProvider provider = provider(client, 2);

        EmbeddingResponse response = provider.embed(request("hello python worker"));

        assertThat(response.getProvider()).isEqualTo("local-python");
    }

    @Test
    void embedShouldFailWhenRuntimeProviderIsBlank() {
        LocalEmbeddingWorkerResponse workerResponse = responseWithProvider("", List.of(data(0, List.of(0.1, 0.2))));
        workerResponse.setProvider("   ");
        LocalEmbeddingWorkerProvider provider = provider(
                new FakeLocalEmbeddingWorkerClient(workerResponse),
                2
        );

        assertThatThrownBy(() -> provider.embed(request("hello")))
                .isInstanceOf(EmbeddingProviderException.class)
                .hasMessageContaining("provider must not be blank");
    }

    @Test
    void embedShouldFailWhenRuntimeProviderIsUnsupported() {
        LocalEmbeddingWorkerProvider provider = provider(
                new FakeLocalEmbeddingWorkerClient(
                        responseWithProvider("openai", List.of(data(0, List.of(0.1, 0.2))))
                ),
                2
        );

        assertThatThrownBy(() -> provider.embed(request("hello")))
                .isInstanceOf(EmbeddingProviderException.class)
                .hasMessageContaining("not supported");
    }

    @Test
    void embedShouldCreateRequestAndParseSingleEmbeddingResponse() {
        FakeLocalEmbeddingWorkerClient client = new FakeLocalEmbeddingWorkerClient(response(List.of(
                data(0, List.of(0.1, 0.2, 0.3))
        )));
        LocalEmbeddingWorkerProvider provider = provider(client, 3);

        EmbeddingResponse response = provider.embed(request("hello local worker"));

        assertThat(client.requests).hasSize(1);
        assertThat(client.requests.getFirst().getModel()).isEqualTo("sentence-transformers/test-model");
        assertThat(client.requests.getFirst().getInput()).containsExactly("hello local worker");
        assertThat(response.getProvider()).isEqualTo("local-worker");
        assertThat(response.getModel()).isEqualTo("sentence-transformers/test-model");
        assertThat(response.getDimension()).isEqualTo(3);
        assertThat(response.getDistanceMetric()).isEqualTo("COSINE");
        assertThat(response.getVector()).containsExactly(0.1, 0.2, 0.3);
    }

    @Test
    void embedBatchShouldReturnVectorsInResponseIndexOrder() {
        FakeLocalEmbeddingWorkerClient client = new FakeLocalEmbeddingWorkerClient(response(List.of(
                data(1, List.of(0.4, 0.5)),
                data(0, List.of(0.1, 0.2))
        )));
        LocalEmbeddingWorkerProvider provider = provider(client, 2);

        List<EmbeddingResponse> responses = provider.embedBatch(List.of(
                request("first"),
                request("second")
        ));

        assertThat(client.requests.getFirst().getInput()).containsExactly("first", "second");
        assertThat(responses).hasSize(2);
        assertThat(responses.get(0).getVector()).containsExactly(0.1, 0.2);
        assertThat(responses.get(1).getVector()).containsExactly(0.4, 0.5);
        assertThat(responses).allSatisfy(response -> {
            assertThat(response.getProvider()).isEqualTo("local-worker");
            assertThat(response.getModel()).isEqualTo("sentence-transformers/test-model");
            assertThat(response.getDimension()).isEqualTo(2);
        });
    }

    @Test
    void embedBatchShouldReturnEmptyListWhenInputIsEmpty() {
        FakeLocalEmbeddingWorkerClient client = new FakeLocalEmbeddingWorkerClient(response(List.of()));
        LocalEmbeddingWorkerProvider provider = provider(client, 2);

        assertThat(provider.embedBatch(List.of())).isEmpty();
        assertThat(client.requests).isEmpty();
    }

    @Test
    void embedShouldFailWhenResponseDataIsEmpty() {
        LocalEmbeddingWorkerProvider provider = provider(
                new FakeLocalEmbeddingWorkerClient(response(List.of())),
                2
        );

        assertThatThrownBy(() -> provider.embed(request("hello")))
                .isInstanceOf(EmbeddingProviderException.class)
                .hasMessageContaining("data");
    }

    @Test
    void embedShouldFailWhenVectorIsEmpty() {
        LocalEmbeddingWorkerProvider provider = provider(
                new FakeLocalEmbeddingWorkerClient(response(List.of(data(0, List.of())))),
                2
        );

        assertThatThrownBy(() -> provider.embed(request("hello")))
                .isInstanceOf(EmbeddingProviderException.class)
                .hasMessageContaining("vector");
    }

    @Test
    void embedBatchShouldFailWhenResponseCountDoesNotMatchInputCount() {
        LocalEmbeddingWorkerProvider provider = provider(
                new FakeLocalEmbeddingWorkerClient(response(List.of(data(0, List.of(0.1, 0.2))))),
                2
        );

        assertThatThrownBy(() -> provider.embedBatch(List.of(request("one"), request("two"))))
                .isInstanceOf(EmbeddingProviderException.class)
                .hasMessageContaining("size");
    }

    @Test
    void embedShouldFailWhenResponseDimensionDoesNotMatchConfiguration() {
        LocalEmbeddingWorkerResponse response = response(List.of(data(0, List.of(0.1, 0.2))));
        response.setDimension(3);
        LocalEmbeddingWorkerProvider provider = provider(
                new FakeLocalEmbeddingWorkerClient(response),
                2
        );

        assertThatThrownBy(() -> provider.embed(request("hello")))
                .isInstanceOf(EmbeddingProviderException.class)
                .hasMessageContaining("dimension");
    }

    @Test
    void embedShouldFailWhenVectorDimensionDoesNotMatchConfiguration() {
        LocalEmbeddingWorkerProvider provider = provider(
                new FakeLocalEmbeddingWorkerClient(response(List.of(data(0, List.of(0.1, 0.2, 0.3))))),
                2
        );

        assertThatThrownBy(() -> provider.embed(request("hello")))
                .isInstanceOf(EmbeddingProviderException.class)
                .hasMessageContaining("dimension");
    }

    @Test
    void embedShouldWrapWorkerClientException() {
        FakeLocalEmbeddingWorkerClient client = new FakeLocalEmbeddingWorkerClient(null);
        client.exception = new IllegalStateException("worker unavailable");
        LocalEmbeddingWorkerProvider provider = provider(client, 2);

        assertThatThrownBy(() -> provider.embed(request("hello")))
                .isInstanceOf(EmbeddingProviderException.class)
                .hasMessageContaining("request failed")
                .hasCauseInstanceOf(IllegalStateException.class);
    }

    private LocalEmbeddingWorkerProvider provider(
            FakeLocalEmbeddingWorkerClient client,
            int dimension
    ) {
        return new LocalEmbeddingWorkerProvider(properties(dimension), client);
    }

    private EmbeddingProperties.LocalWorker properties(int dimension) {
        EmbeddingProperties.LocalWorker properties = new EmbeddingProperties.LocalWorker();
        properties.setBaseUrl("http://127.0.0.1:8001");
        properties.setModel("sentence-transformers/test-model");
        properties.setDimension(dimension);
        properties.setTimeoutMs(1000);
        return properties;
    }

    private EmbeddingRequest request(String text) {
        EmbeddingRequest request = new EmbeddingRequest();
        request.setText(text);
        return request;
    }

    private LocalEmbeddingWorkerResponse response(List<LocalEmbeddingWorkerResponse.EmbeddingData> data) {
        return responseWithProvider("local-worker", data);
    }

    private LocalEmbeddingWorkerResponse responseWithProvider(
            String provider,
            List<LocalEmbeddingWorkerResponse.EmbeddingData> data
    ) {
        LocalEmbeddingWorkerResponse response = new LocalEmbeddingWorkerResponse();
        response.setProvider(provider);
        response.setModel("sentence-transformers/test-model");
        response.setDimension(data.isEmpty() ? 0 : data.getFirst().getEmbedding().size());
        response.setData(data);
        return response;
    }

    private LocalEmbeddingWorkerResponse.EmbeddingData data(Integer index, List<Double> vector) {
        LocalEmbeddingWorkerResponse.EmbeddingData data = new LocalEmbeddingWorkerResponse.EmbeddingData();
        data.setIndex(index);
        data.setEmbedding(vector);
        return data;
    }

    private static class FakeLocalEmbeddingWorkerClient implements LocalEmbeddingWorkerClient {

        private final LocalEmbeddingWorkerResponse response;

        private final List<LocalEmbeddingWorkerRequest> requests = new ArrayList<>();

        private RuntimeException exception;

        private FakeLocalEmbeddingWorkerClient(LocalEmbeddingWorkerResponse response) {
            this.response = response;
        }

        @Override
        public LocalEmbeddingWorkerResponse createEmbeddings(
                LocalEmbeddingWorkerRequest request,
                EmbeddingProperties.LocalWorker properties
        ) {
            requests.add(request);
            if (exception != null) {
                throw exception;
            }
            return response;
        }
    }
}
