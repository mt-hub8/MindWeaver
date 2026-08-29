package com.tuoman.ai_task_orchestrator.llm;

/**
 * V1.0 早期 Task 执行使用的 LLM client 抽象。
 *
 * TaskExecutionService 只依赖这个接口，具体实现可以是 mock 或后续真实 provider。
 * 这让状态机、retry、attempt 逻辑不和模型厂商 SDK 直接绑定。
 */
public interface LlmClient {

    LlmResponse generate(LlmRequest request);
}
