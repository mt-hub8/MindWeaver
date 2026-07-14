package com.tuoman.ai_task_orchestrator.llm;

/**
 * LLM provider 抽象。
 *
 * RAG Answer、Agent Task 和摘要工具只依赖该接口生成文本；
 * 具体实现可以是 mock、本地 Python/Ollama worker 或 OpenAI-compatible provider。
 *
 * LLM provider 与 Embedding provider 分离，避免回答生成模型切换影响向量索引身份。
 */
public interface LlmProvider {

    LlmGenerateResult generate(String systemPrompt, String userPrompt, LlmGenerateOptions options);

    String provider();

    String defaultModel();
}
