package com.tuoman.ai_task_orchestrator.embedding;

import com.tuoman.ai_task_orchestrator.modelprovider.ModelProviderType;
import com.tuoman.ai_task_orchestrator.modelprovider.ResolvedModelProvider;
import com.tuoman.ai_task_orchestrator.service.ModelProviderSelectionService;
import lombok.RequiredArgsConstructor;
import org.springframework.context.annotation.Primary;
import org.springframework.stereotype.Component;

import java.util.List;

@Component
@Primary
@RequiredArgsConstructor
/**
 * 运行时 Embedding provider 路由器。
 *
 * V10 模型供应商配置之后，默认 embedding 可以指向 mock、Ollama/local worker、
 * OpenAI-compatible 或自定义 OpenAI-compatible provider。
 *
 * 关键约束：这里决定“用哪个模型生成向量”，模型或 dimension 改变后，已有文档通常需要 reindex。
 */
public class RoutingEmbeddingProvider implements EmbeddingProvider {

    private final ModelProviderSelectionService selectionService;

    private final MockEmbeddingClient mockEmbeddingClient;

    private final LocalEmbeddingWorkerProvider localEmbeddingWorkerProvider;

    private final RuntimeOpenAiEmbeddingService runtimeOpenAiEmbeddingService;

    private final EmbeddingProperties embeddingProperties;

    private final LocalEmbeddingWorkerClient localEmbeddingWorkerClient;

    @Override
    public EmbeddingResponse embed(EmbeddingRequest request) {
        return embedBatch(List.of(request == null ? new EmbeddingRequest() : request)).getFirst();
    }

    @Override
    public List<EmbeddingResponse> embedBatch(List<EmbeddingRequest> requests) {
        // 每次调用都解析当前默认 provider，但只做路由，不修改默认配置。
        // connection test 和 provider 切换由 model provider service 负责。
        ResolvedModelProvider resolved = selectionService.resolveDefaultEmbedding();
        selectionService.ensureEnabled(resolved);

        return switch (resolved.getProviderType()) {
            case MOCK -> requests.stream().map(mockEmbeddingClient::embed).toList();
            case OLLAMA -> embedViaWorker(requests, resolved);
            case OPENAI_COMPATIBLE, CUSTOM_OPENAI_COMPATIBLE ->
                    runtimeOpenAiEmbeddingService.embedBatch(requests, resolved);
        };
    }

    private List<EmbeddingResponse> embedViaWorker(List<EmbeddingRequest> requests, ResolvedModelProvider resolved) {
        List<String> inputs = requests.stream()
                .map(r -> r == null || r.getText() == null ? "" : r.getText())
                .toList();
        String model = resolved.getEmbeddingModel() != null && !resolved.getEmbeddingModel().isBlank()
                ? resolved.getEmbeddingModel()
                : embeddingProperties.getLocalWorker().getModel();
        LocalEmbeddingWorkerRequest workerRequest = new LocalEmbeddingWorkerRequest(model, inputs);
        LocalEmbeddingWorkerResponse response = localEmbeddingWorkerClient.createEmbeddings(
                workerRequest,
                embeddingProperties.getLocalWorker()
        );
        return response.getData().stream()
                .map(data -> new EmbeddingResponse(
                        response.getProvider() == null ? LocalEmbeddingWorkerProvider.PROVIDER : response.getProvider(),
                        response.getModel() == null ? model : response.getModel(),
                        data.getEmbedding().size(),
                        LocalEmbeddingWorkerProvider.DISTANCE_METRIC,
                        data.getEmbedding()
                ))
                .toList();
    }

    @Override
    public String provider() {
        ResolvedModelProvider resolved = selectionService.resolveDefaultEmbedding();
        if (resolved.getProviderType() == ModelProviderType.MOCK) {
            return MockEmbeddingClient.PROVIDER;
        }
        if (resolved.getProviderType() == ModelProviderType.OLLAMA) {
            return LocalEmbeddingWorkerProvider.PROVIDER;
        }
        return RuntimeOpenAiEmbeddingService.PROVIDER;
    }

    @Override
    public String runtimeProvider() {
        ResolvedModelProvider resolved = selectionService.resolveDefaultEmbedding();
        if (resolved.getProviderType() == ModelProviderType.OLLAMA) {
            return LocalEmbeddingWorkerProvider.RUNTIME_PROVIDER_OLLAMA;
        }
        return provider();
    }

    @Override
    public String model() {
        ResolvedModelProvider resolved = selectionService.resolveDefaultEmbedding();
        if (resolved.getEmbeddingModel() != null && !resolved.getEmbeddingModel().isBlank()) {
            return resolved.getEmbeddingModel();
        }
        return localEmbeddingWorkerProvider.model();
    }

    @Override
    public int dimension() {
        ResolvedModelProvider resolved = selectionService.resolveDefaultEmbedding();
        if (resolved.getEmbeddingDimension() != null && resolved.getEmbeddingDimension() > 0) {
            return resolved.getEmbeddingDimension();
        }
        return localEmbeddingWorkerProvider.dimension();
    }
}
