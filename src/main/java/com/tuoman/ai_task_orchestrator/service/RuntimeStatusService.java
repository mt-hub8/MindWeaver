package com.tuoman.ai_task_orchestrator.service;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.tuoman.ai_task_orchestrator.dto.RuntimeStatusResponse;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProperties;
import com.tuoman.ai_task_orchestrator.embedding.MockEmbeddingClient;
import com.tuoman.ai_task_orchestrator.llm.LlmProperties;
import com.tuoman.ai_task_orchestrator.llm.LocalPythonLlmProvider;
import com.tuoman.ai_task_orchestrator.llm.MockLlmProvider;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreProperties;
import lombok.RequiredArgsConstructor;
import org.springframework.core.env.Environment;
import org.springframework.stereotype.Service;
import org.springframework.web.client.ResourceAccessException;
import org.springframework.web.client.RestClient;

import java.util.Arrays;
import java.util.stream.Collectors;

@Service
@RequiredArgsConstructor
/**
 * V8.0 local-ai runtime 状态服务。
 *
 * 汇总当前 profile、LLM/Embedding provider、Python Worker、Ollama 和 VectorStore 状态。
 * 这是运行诊断接口，不会切换 provider，也不会触发 reindex。
 */
public class RuntimeStatusService {

    private static final String DEFAULT_OLLAMA_BASE_URL = "http://127.0.0.1:11434";

    private static final int PROBE_TIMEOUT_MS = 3000;

    private final Environment environment;

    private final EmbeddingProperties embeddingProperties;

    private final LlmProperties llmProperties;

    private final VectorStoreProperties vectorStoreProperties;

    private final RestClient.Builder restClientBuilder;

    private final ObjectMapper objectMapper;

    public RuntimeStatusResponse getStatus() {
        // default mock profile 是开发测试兜底；local-ai profile 才会依赖 Python Worker + Ollama。
        // 状态探测失败只影响诊断展示，不应伪造模型可用或修改默认配置。
        String activeProfile = resolveActiveProfile();
        boolean mockEmbedding = isMockEmbedding();
        boolean mockLlm = isMockLlm();
        String pythonWorkerBaseUrl = normalizeBaseUrl(llmProperties.getBaseUrl());
        String ollamaBaseUrl = DEFAULT_OLLAMA_BASE_URL;

        Boolean pythonWorkerReachable = null;
        Boolean ollamaReachable = null;
        String statusMessage;

        if (mockEmbedding && mockLlm) {
            statusMessage = "当前使用 mock 模型，仅用于开发测试。启动 local-ai profile 并运行 Python Worker + Ollama 可体验本地真实模型。";
            pythonWorkerReachable = false;
            ollamaReachable = false;
        } else if (mockEmbedding || mockLlm) {
            statusMessage = "当前为混合配置：部分组件使用 mock，部分使用本地运行时。建议在 local-ai profile 下统一使用本地模型。";
            WorkerProbe probe = probePythonWorker(pythonWorkerBaseUrl);
            pythonWorkerReachable = probe.reachable();
            ollamaReachable = probe.ollamaReachable();
            if (probe.ollamaBaseUrl() != null) {
                ollamaBaseUrl = probe.ollamaBaseUrl();
            }
        } else {
            WorkerProbe probe = probePythonWorker(pythonWorkerBaseUrl);
            pythonWorkerReachable = probe.reachable();
            ollamaReachable = probe.ollamaReachable();
            if (probe.ollamaBaseUrl() != null) {
                ollamaBaseUrl = probe.ollamaBaseUrl();
            }
            if (Boolean.TRUE.equals(pythonWorkerReachable) && Boolean.TRUE.equals(ollamaReachable)) {
                statusMessage = "当前使用本地 Ollama 模型，Python Worker 与 Ollama 连接正常。";
            } else if (Boolean.TRUE.equals(pythonWorkerReachable)) {
                statusMessage = "Python Worker 已连接，但 Ollama 暂不可用。请确认 Ollama 已启动。";
            } else {
                statusMessage = "本地 Python Worker 暂不可用。请启动 workers/ai-runtime-worker 并确认端口可访问。";
            }
        }

        return new RuntimeStatusResponse(
                activeProfile,
                embeddingProperties.getProvider(),
                resolveEmbeddingModel(),
                embeddingProperties.getLocalWorker().getDimension(),
                llmProperties.getProvider(),
                llmProperties.getModel(),
                pythonWorkerBaseUrl,
                pythonWorkerReachable,
                ollamaBaseUrl,
                ollamaReachable,
                vectorStoreProperties.getProvider(),
                statusMessage
        );
    }

    private String resolveEmbeddingModel() {
        if (MockEmbeddingClient.PROVIDER.equalsIgnoreCase(embeddingProperties.getProvider())) {
            return MockEmbeddingClient.DEFAULT_MODEL;
        }
        if ("openai".equalsIgnoreCase(embeddingProperties.getProvider())) {
            return embeddingProperties.getOpenai().getModel();
        }
        return embeddingProperties.getLocalWorker().getModel();
    }

    private String resolveActiveProfile() {
        String[] profiles = environment.getActiveProfiles();
        if (profiles.length == 0) {
            return "default";
        }
        return Arrays.stream(profiles).collect(Collectors.joining(","));
    }

    private boolean isMockEmbedding() {
        return MockEmbeddingClient.PROVIDER.equalsIgnoreCase(embeddingProperties.getProvider());
    }

    private boolean isMockLlm() {
        return MockLlmProvider.PROVIDER.equalsIgnoreCase(llmProperties.getProvider());
    }

    private WorkerProbe probePythonWorker(String baseUrl) {
        try {
            RestClient client = restClientBuilder
                    .baseUrl(baseUrl)
                    .build();
            String body = client.get()
                    .uri("/health")
                    .retrieve()
                    .body(String.class);
            if (body == null || body.isBlank()) {
                return WorkerProbe.unreachable();
            }
            JsonNode json = objectMapper.readTree(body);
            boolean reachable = true;
            boolean ollama = json.path("ollamaReachable").asBoolean(false);
            String ollamaBase = json.path("ollamaBaseUrl").asText(null);
            return new WorkerProbe(reachable, ollama, ollamaBase);
        } catch (ResourceAccessException | IllegalArgumentException exception) {
            return WorkerProbe.unreachable();
        } catch (Exception exception) {
            return WorkerProbe.unreachable();
        }
    }

    private String normalizeBaseUrl(String baseUrl) {
        if (baseUrl == null || baseUrl.isBlank()) {
            return "http://127.0.0.1:8001";
        }
        return baseUrl.endsWith("/") ? baseUrl.substring(0, baseUrl.length() - 1) : baseUrl;
    }

    private record WorkerProbe(boolean reachable, boolean ollamaReachable, String ollamaBaseUrl) {
        static WorkerProbe unreachable() {
            return new WorkerProbe(false, false, null);
        }
    }
}
