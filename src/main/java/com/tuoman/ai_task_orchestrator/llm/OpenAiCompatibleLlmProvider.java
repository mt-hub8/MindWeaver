package com.tuoman.ai_task_orchestrator.llm;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.modelprovider.ResolvedModelProvider;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Component;

import java.util.List;
import java.util.Map;

@Component
@RequiredArgsConstructor
/**
 * OpenAI-compatible LLM provider。
 *
 * 该适配器把统一的 LlmProvider 调用转换成 OpenAI Chat Completions 风格协议，
 * 支持 OpenAI-compatible 和自定义兼容供应商。
 */
public class OpenAiCompatibleLlmProvider {

    public static final String PROVIDER = "openai-compatible";

    private final OpenAiCompatibleLlmHttpClient httpClient;

    public LlmGenerateResult generate(
            String systemPrompt,
            String userPrompt,
            LlmGenerateOptions options,
            ResolvedModelProvider config
    ) {
        long started = System.currentTimeMillis();
        String model = resolveModel(options, config);

        OpenAiChatRequest request = new OpenAiChatRequest();
        request.setModel(model);
        request.setTemperature(options != null && options.getTemperature() != null ? options.getTemperature() : 0.2);
        request.setMaxTokens(options != null && options.getMaxTokens() != null ? options.getMaxTokens() : 1200);
        request.setMessages(List.of(
                new OpenAiChatMessage("system", systemPrompt == null ? "" : systemPrompt),
                new OpenAiChatMessage("user", userPrompt == null ? "" : userPrompt)
        ));

        try {
            // config 中的 API Key 由 HTTP client 使用，不应在 LlmGenerateResult metadata 中明文返回。
            OpenAiChatResponse response = httpClient.createChatCompletion(request, config);
            String content = extractContent(response);
            if (content == null || content.isBlank()) {
                return LlmGenerateResult.failure(PROVIDER, model, "OpenAI-compatible LLM returned empty content", elapsed(started));
            }
            Integer inputTokens = response.getUsage() == null ? null : response.getUsage().getPromptTokens();
            Integer outputTokens = response.getUsage() == null ? null : response.getUsage().getCompletionTokens();
            return LlmGenerateResult.success(
                    content,
                    PROVIDER,
                    model,
                    inputTokens,
                    outputTokens,
                    elapsed(started),
                    "stop",
                    Map.of("source", "openai-compatible", "configId", config.getConfigId())
            );
        } catch (BusinessException exception) {
            throw exception;
        } catch (Exception exception) {
            throw BusinessException.llmProviderError(
                    exception.getMessage() == null ? "OpenAI-compatible LLM request failed" : exception.getMessage()
            );
        }
    }

    private String extractContent(OpenAiChatResponse response) {
        if (response == null || response.getChoices() == null || response.getChoices().isEmpty()) {
            return null;
        }
        OpenAiChatMessage message = response.getChoices().getFirst().getMessage();
        return message == null ? null : message.getContent();
    }

    private String resolveModel(LlmGenerateOptions options, ResolvedModelProvider config) {
        if (options != null && options.getModel() != null && !options.getModel().isBlank()) {
            return options.getModel();
        }
        if (config.getLlmModel() != null && !config.getLlmModel().isBlank()) {
            return config.getLlmModel();
        }
        throw BusinessException.modelProviderInvalid("未配置默认 LLM 模型");
    }

    private long elapsed(long started) {
        return System.currentTimeMillis() - started;
    }
}
