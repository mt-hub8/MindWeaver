package com.tuoman.ai_task_orchestrator.llm;

import com.tuoman.ai_task_orchestrator.modelprovider.ModelProviderType;
import com.tuoman.ai_task_orchestrator.modelprovider.ResolvedModelProvider;
import com.tuoman.ai_task_orchestrator.service.ModelProviderSelectionService;
import lombok.RequiredArgsConstructor;
import org.springframework.context.annotation.Primary;
import org.springframework.stereotype.Component;

@Component
@Primary
@RequiredArgsConstructor
/**
 * LLM 运行时路由器。
 *
 * 根据 V10 ModelProviderSelectionService 的默认配置，把 generate 调用分发到 mock、
 * local Python/Ollama 或 OpenAI-compatible provider。
 */
public class RoutingLlmProvider implements LlmProvider {

    private final ModelProviderSelectionService selectionService;

    private final MockLlmProvider mockLlmProvider;

    private final LocalPythonLlmProvider localPythonLlmProvider;

    private final OpenAiCompatibleLlmProvider openAiCompatibleLlmProvider;

    @Override
    public LlmGenerateResult generate(String systemPrompt, String userPrompt, LlmGenerateOptions options) {
        // 每次生成前解析默认 provider，但不在这里修改默认配置。
        // 供应商切换由配置服务负责，生成链路只消费当前解析结果。
        ResolvedModelProvider resolved = selectionService.resolveDefaultLlm();
        selectionService.ensureEnabled(resolved);
        LlmGenerateOptions effective = options != null ? options : new LlmGenerateOptions();
        if ((effective.getModel() == null || effective.getModel().isBlank())
                && resolved.getLlmModel() != null && !resolved.getLlmModel().isBlank()) {
            effective.setModel(resolved.getLlmModel());
        }

        return switch (resolved.getProviderType()) {
            case MOCK -> mockLlmProvider.generate(systemPrompt, userPrompt, effective);
            case OLLAMA -> localPythonLlmProvider.generate(systemPrompt, userPrompt, effective);
            case OPENAI_COMPATIBLE, CUSTOM_OPENAI_COMPATIBLE ->
                    openAiCompatibleLlmProvider.generate(systemPrompt, userPrompt, effective, resolved);
        };
    }

    @Override
    public String provider() {
        return selectionService.resolveDefaultLlm().getProviderType().name().toLowerCase();
    }

    @Override
    public String defaultModel() {
        ResolvedModelProvider resolved = selectionService.resolveDefaultLlm();
        if (resolved.getLlmModel() != null && !resolved.getLlmModel().isBlank()) {
            return resolved.getLlmModel();
        }
        return localPythonLlmProvider.defaultModel();
    }
}
