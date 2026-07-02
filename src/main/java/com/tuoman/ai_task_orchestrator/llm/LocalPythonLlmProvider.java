package com.tuoman.ai_task_orchestrator.llm;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Component;

import java.util.Map;

@Component
@RequiredArgsConstructor
public class LocalPythonLlmProvider implements LlmProvider {

    public static final String PROVIDER = "local-python";

    private final LlmProperties properties;

    private final LocalPythonLlmHttpClient httpClient;

    @Override
    public LlmGenerateResult generate(String systemPrompt, String userPrompt, LlmGenerateOptions options) {
        long started = System.currentTimeMillis();
        String model = resolveModel(options);

        LocalPythonLlmRequest request = new LocalPythonLlmRequest();
        request.setSystemPrompt(systemPrompt);
        request.setUserPrompt(userPrompt);
        request.setModel(model);
        request.setTemperature(options != null && options.getTemperature() != null
                ? options.getTemperature()
                : properties.getTemperature());
        request.setMaxTokens(options != null && options.getMaxTokens() != null
                ? options.getMaxTokens()
                : properties.getMaxTokens());

        LocalPythonLlmResponse response;
        try {
            response = httpClient.generate(request, properties);
        } catch (BusinessException exception) {
            throw exception;
        } catch (RuntimeException exception) {
            throw BusinessException.aiRuntimeUnavailable(
                    exception.getMessage() == null ? "Python LLM worker call failed" : exception.getMessage()
            );
        }

        if (response == null || response.getContent() == null || response.getContent().isBlank()) {
            return LlmGenerateResult.failure(
                    PROVIDER,
                    model,
                    "Python LLM worker returned empty content",
                    elapsed(started)
            );
        }

        Integer inputTokens = response.getUsage() == null ? null : response.getUsage().getInputTokens();
        Integer outputTokens = response.getUsage() == null ? null : response.getUsage().getOutputTokens();
        return LlmGenerateResult.success(
                response.getContent(),
                response.getProvider() == null ? PROVIDER : response.getProvider(),
                response.getModel() == null ? model : response.getModel(),
                inputTokens,
                outputTokens,
                response.getLatencyMs() == null ? elapsed(started) : response.getLatencyMs(),
                response.getFinishReason(),
                Map.of(
                        "source", "local-python-worker",
                        "ollamaProvider", response.getProvider() == null ? PROVIDER : response.getProvider()
                )
        );
    }

    @Override
    public String provider() {
        return PROVIDER;
    }

    @Override
    public String defaultModel() {
        return properties.getModel();
    }

    private String resolveModel(LlmGenerateOptions options) {
        if (options != null && options.getModel() != null && !options.getModel().isBlank()) {
            return options.getModel();
        }
        return properties.getModel();
    }

    private long elapsed(long started) {
        return System.currentTimeMillis() - started;
    }
}
