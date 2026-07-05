package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.RuntimeTestResponse;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProvider;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingRequest;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingResponse;
import com.tuoman.ai_task_orchestrator.llm.LlmGenerateOptions;
import com.tuoman.ai_task_orchestrator.llm.LlmGenerateResult;
import com.tuoman.ai_task_orchestrator.llm.LlmProvider;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;

@Service
@RequiredArgsConstructor
public class RuntimeTestService {

    private static final String EMBEDDING_PROBE_TEXT = "连接测试";

    private static final String LLM_PROBE_PROMPT = "请回复：连接正常";

    private final EmbeddingProvider embeddingProvider;

    private final LlmProvider llmProvider;

    public RuntimeTestResponse testEmbedding() {
        long start = System.currentTimeMillis();
        try {
            EmbeddingRequest request = new EmbeddingRequest();
            request.setText(EMBEDDING_PROBE_TEXT);
            EmbeddingResponse response = embeddingProvider.embed(request);
            long latency = System.currentTimeMillis() - start;
            if (response == null || response.getVector() == null || response.getVector().isEmpty()) {
                return new RuntimeTestResponse(false, "Embedding 测试失败：未返回向量。", latency, null, null);
            }
            return new RuntimeTestResponse(
                    true,
                    "Embedding 测试成功，向量维度 " + response.getDimension() + "。",
                    latency,
                    response.getProvider(),
                    response.getModel()
            );
        } catch (Exception exception) {
            long latency = System.currentTimeMillis() - start;
            return new RuntimeTestResponse(
                    false,
                    "Embedding 测试失败：" + exception.getMessage(),
                    latency,
                    null,
                    null
            );
        }
    }

    public RuntimeTestResponse testLlm() {
        long start = System.currentTimeMillis();
        try {
            LlmGenerateOptions options = new LlmGenerateOptions();
            options.setMaxTokens(32);
            options.setTemperature(0.0);
            LlmGenerateResult result = llmProvider.generate(
                    "你是连接测试助手。",
                    LLM_PROBE_PROMPT,
                    options
            );
            long latency = System.currentTimeMillis() - start;
            if (result == null || !result.isSuccess()) {
                String message = result != null && result.getErrorMessage() != null
                        ? result.getErrorMessage()
                        : "未知错误";
                return new RuntimeTestResponse(false, "LLM 测试失败：" + message, latency, null, null);
            }
            return new RuntimeTestResponse(
                    true,
                    "LLM 测试成功。",
                    latency,
                    result.getProvider(),
                    result.getModel()
            );
        } catch (Exception exception) {
            long latency = System.currentTimeMillis() - start;
            return new RuntimeTestResponse(
                    false,
                    "LLM 测试失败：" + exception.getMessage(),
                    latency,
                    null,
                    null
            );
        }
    }
}
