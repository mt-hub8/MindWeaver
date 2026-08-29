package com.tuoman.ai_task_orchestrator.modelprovider;

import com.tuoman.ai_task_orchestrator.dto.ModelProviderPresetResponse;

import java.util.List;

public final class ModelProviderPresetCatalog {

    private ModelProviderPresetCatalog() {
    }

    public static List<ModelProviderPresetResponse> all() {
        return List.of(
                new ModelProviderPresetResponse(
                        "ollama-local",
                        ModelProviderType.OLLAMA,
                        "本地 Ollama",
                        "http://127.0.0.1:11434",
                        "qwen2.5:7b",
                        "qwen3-embedding:0.6b",
                        1024,
                        "通过 Python Worker 调用本机 Ollama，本地模型不需要 API Key。"
                ),
                new ModelProviderPresetResponse(
                        "openai-custom",
                        ModelProviderType.CUSTOM_OPENAI_COMPATIBLE,
                        "自定义 OpenAI-compatible API",
                        "",
                        "",
                        "",
                        null,
                        "填写兼容 OpenAI 协议的 Base URL、API Key 与模型名称。"
                ),
                new ModelProviderPresetResponse(
                        "deepseek-compatible",
                        ModelProviderType.OPENAI_COMPATIBLE,
                        "DeepSeek（OpenAI-compatible）",
                        "https://api.deepseek.com/v1",
                        "deepseek-chat",
                        "",
                        null,
                        "DeepSeek 使用 OpenAI-compatible 协议。请自行填写 API Key，不会预置或调用真实 Key。"
                ),
                new ModelProviderPresetResponse(
                        "qwen-compatible",
                        ModelProviderType.OPENAI_COMPATIBLE,
                        "Qwen / DashScope（OpenAI-compatible）",
                        "https://dashscope.aliyuncs.com/compatible-mode/v1",
                        "qwen-plus",
                        "text-embedding-v3",
                        1024,
                        "通义千问兼容模式。V10.0 不做 DashScope 特殊协议，仅兼容 OpenAI API 形态。"
                ),
                new ModelProviderPresetResponse(
                        "siliconflow-compatible",
                        ModelProviderType.OPENAI_COMPATIBLE,
                        "SiliconFlow（OpenAI-compatible）",
                        "https://api.siliconflow.cn/v1",
                        "",
                        "",
                        null,
                        "SiliconFlow 兼容 OpenAI API。请填写 API Key 与模型名称。"
                )
        );
    }
}
