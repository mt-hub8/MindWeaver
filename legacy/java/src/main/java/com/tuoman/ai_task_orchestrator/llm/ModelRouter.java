package com.tuoman.ai_task_orchestrator.llm;

import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;

import java.util.Set;

@Slf4j
@Component
/**
 * V1.3 早期模型路由器。
 *
 * Task 可以请求模型，但执行链路通过 router 解析为受支持模型。
 * 不支持的模型回退到默认 mock，保证本地演示链路可运行；真实 provider 路由由后续供应商配置扩展。
 */
public class ModelRouter {

    private static final String DEFAULT_MODEL = "mock-llm";

    private static final Set<String> SUPPORTED_MODELS = Set.of(
            "mock-llm",
            "mock-fast",
            "mock-smart"
    );

    public String route(String requestedModel) {
        if (requestedModel == null || requestedModel.isBlank()) {
            return DEFAULT_MODEL;
        }

        if (SUPPORTED_MODELS.contains(requestedModel)) {
            return requestedModel;
        }

        log.warn("Unsupported requested model, fallback to default model, requestedModel={}, defaultModel={}",
                requestedModel,
                DEFAULT_MODEL);
        return DEFAULT_MODEL;
    }
}
