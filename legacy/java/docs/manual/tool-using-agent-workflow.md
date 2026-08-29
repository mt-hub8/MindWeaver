# V7.0 Tool-Using Agent Workflow（可调用工具的 Agent 工作流）

## 目标

V7.0 将 Agent Task 从「一次 RAG + 一次 LLM」升级为**受控的固定工具工作流**：

1. 检索知识库（`knowledge_search`）
2. 总结检索结果（`context_summary`）
3. 生成最终报告（`final_report`）

本阶段**不做**复杂 ReAct 循环、自主 planner、多 Agent、外部危险工具。

## Tool-Using Agent 是什么

Agent Task 创建后，系统会生成固定的三步执行计划，按顺序调用内置 Java 工具，保存每一步的输入/输出与状态，最后基于工具结果调用 LLM 生成最终报告（无上下文时不调用 LLM）。

## Tool Registry 设计

- `AgentTool`：工具抽象（`toolName`、`displayName`、`description`、`inputSchema`、`execute`）
- `AgentToolRegistry`：注册并查找内置工具
- API：`GET /agent/tools` 返回可用工具列表（中文展示名 + schema）

## 当前内置工具

| 技术名称 | 中文名 | 用途 |
|---|---|---|
| `knowledge_search` | 检索知识库 | 在 collection / 全库范围内检索片段与 citations |
| `context_summary` | 总结检索结果 | 对检索片段做结构化摘要 |
| `collection_overview` | 分析知识库分组 | 统计分组文档与 chunk 数量（可选调用，不在默认三步计划中） |

## Fixed Step Plan

默认计划（任务创建时写入 `agent_task_step`）：

1. **检索知识库**（TOOL_CALL）
2. **总结检索结果**（TOOL_CALL）
3. **生成最终报告**（FINAL_REPORT）

## Agent Task Step 状态

| 状态 | 中文展示 |
|---|---|
| PENDING | 待执行 |
| RUNNING | 执行中 |
| COMPLETED | 已完成 |
| FAILED | 执行失败 |
| SKIPPED | 已跳过 |

## Tool Execution Result

每一步保存 `input_json` / `output_json`（过大内容会截断）。页面可查看「工具输入 / 工具输出」。

## No-Context 行为

若 `knowledge_search` 无可用上下文：

- Step 1 标记 `noContext` 并完成
- Step 2 跳过或输出「无可总结内容」
- Step 3 使用确定性文案完成，**不调用 LLM**
- 任务仍为 COMPLETED，结果为：「当前范围内没有可用于执行任务的知识库内容。」

## Final Report 生成

有上下文时，最终报告 prompt 基于：

- 任务目标
- 知识库范围
- `knowledge_search` 结果
- `context_summary` 结果
- citations

System prompt 要求：不得编造工具结果之外的信息。

## Citations 贯通

`knowledge_search` 产生的 citations 写入 `agent_task_citation`，并在任务详情中展示，与最终报告引用索引一致。

## Collection / Lifecycle Filter

- `knowledge_search` 复用 V5.0 scoped retrieval 与 V4.0 lifecycle filter
- 已删除文档、旧 generation chunk、collection 外文档不会进入 citations

## 工具失败处理

- 工具不存在：`AGENT_TOOL_NOT_FOUND`
- 输入非法：`AGENT_TOOL_INPUT_INVALID`
- 工具执行失败：当前 step FAILED，任务 FAILED，事件与 traceId 可追踪

## 默认测试策略

默认 `mvn test` 使用 Mock Embedding / Mock LLM，不依赖 Python worker、Qdrant、Docker 或外部 API。

## 本阶段明确不做

- shell / code execution / browser / external web
- multi-agent / autonomous planner / streaming
- permissions / workspace / 付费计费

## 相关页面与 API

- `/agent-tasks.html`：任务执行计划、工具执行记录、最终报告
- `/agent-tools.html`：可用工具列表
- `GET /agent/tools`
- `GET /agent/tasks/{id}`（含 `steps`）
