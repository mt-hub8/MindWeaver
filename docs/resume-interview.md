# 简历与面试表达文档

## 一、项目一句话描述

AI Task Orchestrator 是一个基于 Spring Boot、RabbitMQ、MySQL 和 Flyway 构建的异步 AI 任务编排系统，支持状态机、事件追踪、失败重试、幂等、取消、超时、Prompt Template 渲染、Mock LLM 调用、结果保存和 LLM usage 记录。

当前仍然使用 `MockLlmClient`，尚未接入真实 OpenAI / Claude / 本地模型 Provider；token usage 是 Mock 估算，不是真实 tokenizer 或真实成本。

## 二、简历 bullet

- 基于 Spring Boot 和 RabbitMQ 实现异步任务调度，将任务创建与后台执行解耦。
- 设计任务状态机和 `task_event`，记录任务生命周期并防止非法流转。
- 实现失败处理、自动重试、入口幂等、协作式取消和超时扫描。
- 设计 `LlmClient` 抽象与 `MockLlmClient`，实现任务执行链路与模型调用解耦。
- 设计 Prompt Template 数据模型与渲染器，将 `default_task_prompt` 接入任务执行链路。
- 保存 `renderedPrompt`、`promptTemplateCode`、`resultContent`、`llmModel`，便于追踪每次任务的实际 Prompt 和输出。
- 为 Mock LLM 调用记录 `llmProvider`、token usage 和 latency，为后续真实模型成本统计打基础。
- 使用 Flyway 管理数据库结构演进，使用 Docker Compose 搭建本地 MySQL / RabbitMQ 环境。

## 三、面试开场

这个项目模拟的是 AI Agent / LLM 平台里的长耗时任务执行系统。它用 Spring Boot 提供 API，用 RabbitMQ 解耦任务创建和执行，用 MySQL 保存任务、事件、Prompt 模板和执行结果，用 Flyway 管理数据库迁移。当前已经完成可靠任务系统底座，并接入了 Mock LLM 执行链路：任务执行时会渲染 `default_task_prompt`，把 `renderedPrompt` 传给 `MockLlmClient`，再保存输出、模型名、provider、token usage 和 latency。它还没有接入真实模型，但已经具备后续接入真实 Provider 所需要的状态管理、失败恢复、Prompt 追踪和 usage 记录基础。

## 四、系统架构

```text
TaskController
-> TaskService
-> RabbitMQ Producer
-> RabbitMQ Consumer
-> TaskExecutionService
-> PromptTemplateRepository
-> PromptTemplateRenderer
-> LlmClient
-> MockLlmClient
-> saveLlmMetadata
-> markTaskSucceeded / markTaskRetryPending / markTaskFailed
```

## 五、技术亮点

- 任务生命周期建模
- 状态机约束
- 事件追踪
- RabbitMQ 异步调度
- 失败恢复和自动重试
- Consumer 入口幂等保护
- 协作式取消
- 超时扫描
- Prompt Template 渲染
- LLM 调用抽象
- Mock Provider
- LLM metadata 与 token usage 记录
- Flyway schema migration

## 六、如何诚实回答“这是 AI 项目吗？”

这个项目当前不是完整的 LLM 应用，也没有调用真实 OpenAI / Claude / 本地模型。它实现的是 AI 任务平台的工程底座，并用 `MockLlmClient` 模拟模型调用，完成 Prompt 渲染、异步执行、失败重试、结果保存和 usage 记录。后续接入真实 Provider 时，可以复用这套任务编排和可观测能力。

## 七、当前边界

尚未实现：

- 真实 OpenAI / Claude / 本地模型 Provider
- API Key 配置
- 真实 tokenizer
- 真实成本统计
- Prompt Template CRUD API
- Streaming Output
- RAG
- Tool Calling
- Agent Runtime
- KV Cache-aware Scheduling

## 八、后续扩展路线

- 接入真实模型 Provider
- 增加 API Key 配置和安全管理
- 接入真实 tokenizer
- 记录真实成本
- Prompt Template CRUD
- Streaming Output
- RAG
- Tool Calling
- Agent Runtime
- Evaluation Harness
