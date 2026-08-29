package com.tuoman.ai_task_orchestrator.embedding;

import java.util.Comparator;
import java.util.List;

/**
 * V2.5.2/V8.0 本地 embedding worker provider。
 *
 * Java 通过 HTTP client 调用 Python worker，worker 再连接本地 runtime/Ollama。
 * 业务层只看到 EmbeddingProvider 契约，避免文档摄入和检索代码直接依赖某个模型运行时。
 *
 * 关键不变量：worker 返回的 provider/model/dimension 必须与配置兼容；
 * dimension mismatch 必须失败，不能自动纠正后写入 vector index。
 */
public class LocalEmbeddingWorkerProvider implements EmbeddingProvider {

    public static final String PROVIDER = "local-worker";

    public static final String RUNTIME_PROVIDER_OLLAMA = "local-ollama";

    public static final String RUNTIME_PROVIDER_PYTHON = "local-python";

    public static final java.util.Set<String> ACCEPTED_RUNTIME_PROVIDERS = java.util.Set.of(
            PROVIDER,
            RUNTIME_PROVIDER_OLLAMA,
            RUNTIME_PROVIDER_PYTHON
    );

    public static final String DISTANCE_METRIC = "COSINE";

    private final EmbeddingProperties.LocalWorker properties;

    private final LocalEmbeddingWorkerClient client;

    public LocalEmbeddingWorkerProvider(
            EmbeddingProperties.LocalWorker properties,
            LocalEmbeddingWorkerClient client
    ) {
        this.properties = properties;
        this.client = client;
    }

    @Override
    public EmbeddingResponse embed(EmbeddingRequest request) {
        return embedBatch(List.of(request == null ? new EmbeddingRequest() : request)).getFirst();
    }

    @Override
    public List<EmbeddingResponse> embedBatch(List<EmbeddingRequest> requests) {
        // batch 请求保持输入顺序，worker 响应按 index 排序还原。
        // 这保证 chunk 与 embedding 一一对应，避免向量写入错配。
        if (requests == null || requests.isEmpty()) {
            return List.of();
        }

        List<String> inputs = requests.stream()
                .map(request -> request == null ? null : request.getText())
                .map(text -> text == null ? "" : text)
                .toList();

        LocalEmbeddingWorkerRequest workerRequest = new LocalEmbeddingWorkerRequest(model(), inputs);
        LocalEmbeddingWorkerResponse response;
        try {
            response = client.createEmbeddings(workerRequest, properties);
        } catch (EmbeddingProviderException exception) {
            throw exception;
        } catch (Exception exception) {
            throw new EmbeddingProviderException("Local embedding worker request failed", exception);
        }

        validateResponse(response, inputs.size());

        return response.getData().stream()
                .sorted(Comparator.comparing(LocalEmbeddingWorkerResponse.EmbeddingData::getIndex))
                .map(data -> toEmbeddingResponse(response, data.getEmbedding()))
                .toList();
    }

    @Override
    public String provider() {
        return PROVIDER;
    }

    @Override
    public String runtimeProvider() {
        return RUNTIME_PROVIDER_OLLAMA;
    }

    @Override
    public String model() {
        return properties.getModel();
    }

    @Override
    public int dimension() {
        return properties.getDimension() == null ? 0 : properties.getDimension();
    }

    private EmbeddingResponse toEmbeddingResponse(
            LocalEmbeddingWorkerResponse response,
            List<Double> vector
    ) {
        validateVector(vector);
        return new EmbeddingResponse(
                normalizeProvider(response.getProvider()),
                normalizeModel(response.getModel()),
                vector.size(),
                DISTANCE_METRIC,
                vector
        );
    }

    private void validateResponse(LocalEmbeddingWorkerResponse response, int expectedCount) {
        // 本地 runtime 可以是 Python worker 或 Ollama，但维度和模型身份必须明确。
        // 模型切换导致维度变化时，应通过 reindex 处理，而不是混写旧索引。
        if (response == null || response.getData() == null || response.getData().isEmpty()) {
            throw new EmbeddingProviderException("Local embedding worker response data must not be empty");
        }
        if (response.getProvider() == null || response.getProvider().isBlank()) {
            throw new EmbeddingProviderException("Local embedding worker response provider must not be blank");
        }
        if (!isAcceptedRuntimeProvider(response.getProvider())) {
            throw new EmbeddingProviderException("Local embedding worker response provider is not supported: "
                    + response.getProvider());
        }
        if (response.getModel() == null || response.getModel().isBlank()) {
            throw new EmbeddingProviderException("Local embedding worker response model must not be blank");
        }
        if (response.getData().size() != expectedCount) {
            throw new EmbeddingProviderException("Local embedding worker response size does not match input size");
        }
        if (response.getDimension() != null && properties.getDimension() != null
                && response.getDimension() > 0 && properties.getDimension() > 0
                && !response.getDimension().equals(properties.getDimension())) {
            throw new EmbeddingProviderException("Local embedding worker response dimension mismatch");
        }
    }

    private void validateVector(List<Double> vector) {
        if (vector == null || vector.isEmpty()) {
            throw new EmbeddingProviderException("Local embedding worker vector must not be empty");
        }
        if (properties.getDimension() != null && properties.getDimension() > 0
                && vector.size() != properties.getDimension()) {
            throw new EmbeddingProviderException("Local embedding worker vector dimension mismatch");
        }
    }

    private String normalizeProvider(String responseProvider) {
        return responseProvider == null || responseProvider.isBlank() ? PROVIDER : responseProvider;
    }

    private String normalizeModel(String responseModel) {
        return responseModel == null || responseModel.isBlank() ? model() : responseModel;
    }

    static boolean isAcceptedRuntimeProvider(String runtimeProvider) {
        return runtimeProvider != null && ACCEPTED_RUNTIME_PROVIDERS.contains(runtimeProvider);
    }

    static boolean isProviderCompatible(String routeProvider, String runtimeProvider) {
        if (runtimeProvider == null || runtimeProvider.isBlank()) {
            return false;
        }
        if (runtimeProvider.equals(routeProvider)) {
            return true;
        }
        return PROVIDER.equals(routeProvider) && isAcceptedRuntimeProvider(runtimeProvider);
    }
}
