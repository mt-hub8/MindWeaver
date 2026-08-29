# V9.0 个人 AI 知识工作台（Local Personal Knowledge Workspace）

## 产品定位

**Personal AI Knowledge Workspace / 个人 AI 知识工作台**

一句话说明：一个本地优先的个人 AI 应用，用户可以上传自己的资料，配置本地或外部模型，构建个人知识库，并让 AI 基于自己的知识执行问答、总结、报告生成和任务编排。

本项目底层仍具备 **AI Task Orchestration**（任务编排、异步流水线、Tool Workflow）能力，但 V9.0 将用户可见产品收敛为个人本地知识工作台体验，而非企业级演示系统。

## 为什么调整定位

早期版本强调「企业级 AI Task Orchestrator / RAG 质量调试」，适合工程演示与面试展示。V9.0 起，产品重心转向：

- 个人用户可理解的入口与中文界面；
- 本地 Ollama + Python Worker 的真实模型体验；
- 技术细节默认折叠，减少 engineering noise；
- Windows 一键检查与启动脚本；
- 为后续「单用户本地数据目录」做规划说明。

企业级能力（Outbox、评估 Harness、Hybrid 检索等）仍保留在代码与历史文档中，但不再作为首页主叙事。

## 用户能做什么

| 能力 | 说明 |
|------|------|
| 构建个人知识库 | 上传 `.txt` / `.md` / 文本型 PDF，异步切块与向量化 |
| 知识库分组 | 按主题管理文档，问答/任务可限定范围 |
| 知识库问答 | 即时提问，查看回答与引用来源 |
| AI 任务报告 | 检索 + 工具 + LLM 生成结构化报告 |
| 模型设置 | 查看 mock / local-ai 状态，测试 Embedding 与 LLM 连接 |
| 系统设置 | 只读查看运行环境与本地数据目录规划 |
| 文档处理分析 | 查看摄入成功率与失败原因（工程向） |

## 当前核心页面

| 页面 | 路径 |
|------|------|
| 首页 | `/` |
| 文档管理 | `/documents.html` |
| 知识库分组 | `/collections.html` |
| 知识库问答 | `/ask.html` |
| AI 任务 | `/agent-tasks.html` |
| 模型设置 | `/model-settings.html` |
| 系统设置 | `/settings.html` |
| 文档处理分析 | `/ingestion-analytics.html` |

主导航统一为：首页 · 文档管理 · 知识库分组 · 知识库问答 · AI 任务 · 模型设置 · 系统设置。

## 模型设置说明

访问 **模型设置**（`/model-settings.html`）可查看：

- 当前运行模式（`default` / `local-ai` 等）
- Embedding / LLM 提供方与模型名
- Python Worker 与 Ollama 地址及连接状态
- **测试 Embedding** / **测试 LLM** 按钮（调用 `POST /runtime/test/embedding` 与 `POST /runtime/test/llm`）

只读状态 API：`GET /runtime/status`

| 字段 | 说明 |
|------|------|
| activeProfile | 当前 Spring profile |
| embeddingProvider / embeddingModel / embeddingDimension | Embedding 配置 |
| llmProvider / llmModel | LLM 配置 |
| pythonWorkerBaseUrl / pythonWorkerReachable | Worker 地址与可达性 |
| ollamaBaseUrl / ollamaReachable | Ollama 地址与可达性 |
| statusMessage | 中文状态说明 |

**mock 模式**（默认测试）：不探测外部 Worker/Ollama，显示「当前使用 mock 模型，仅用于开发测试」。

**local-ai 模式**：探测 Worker `/health`，展示 Ollama 连接情况。详见 [real-local-ai-runtime-with-ollama.md](./real-local-ai-runtime-with-ollama.md)。

本阶段**不做**：API Key 存储、云模型接入、Provider CRUD。

## 本地数据目录规划

建议的 Windows 本地数据目录（规划，尚未迁移）：

```
%APPDATA%\PersonalAIKnowledgeWorkspace\
```

当前开发模式仍使用项目配置的 MySQL、Qdrant、RabbitMQ 与 `workers/ai-runtime-worker`。后续单用户模式会逐步将配置与数据收敛到统一目录。**V9.0 不实际迁移数据。**

在 **系统设置**（`/settings.html`）可查看产品名称、运行模式、组件地址与垃圾箱/缓存说明。

## 一键启动脚本说明

目录：`scripts/windows/`

| 脚本 | 用途 |
|------|------|
| `check-env.ps1` | 检查 Java、Python、Ollama、推荐模型、端口 8001/8080/11434 |
| `start-local.ps1` | 启动 Worker + Spring Boot（local-ai）并打开浏览器 |
| `stop-local.ps1` | 停止本脚本记录的进程（不误杀其他 Java/Python） |
| `README.md` | 中文使用说明与常见问题 |

## 技术详情折叠说明

V9.0 各页面默认展示中文用户语义（已完成、处理中、引用来源等）。以下字段折叠在「查看技术详情」中：

- eventType、metadata、traceId
- provider、model、dimension、tokens、latency
- step input_json / output_json
- retrieval / generation 原始 JSON

涉及页面：文档管理（处理记录时间线）、知识库问答、AI 任务详情。

## 当前不做的边界

- 登录 / 多用户 / Workspace 隔离
- API Key 加密存储与云模型（OpenAI / Claude / Gemini / DeepSeek）
- SQLite 替换 MySQL、embedded 队列替换 RabbitMQ
- 批量上传、垃圾箱自动物理清理、RAG 评分系统、Skill / 多 Agent
- 桌面应用打包、React/Vue 前端框架
- 修改 `docker-compose.yml` 与 `.github/**`

## 后续路线图

1. 将数据与配置迁移到 `%APPDATA%\PersonalAIKnowledgeWorkspace\`
2. 模型设置支持更多本地模型预设与健康检查
3. 简化依赖启动（仍保持 MySQL + RabbitMQ + Qdrant 架构）
4. 个人向 onboarding 与示例知识库
5. 可选：打包为 Windows 本地服务

## 历史版本文档

- [V3.2 文档上传](./upload-to-ask.md)
- [V4.0 知识库生命周期](./knowledge-base-lifecycle-management.md)
- [V5.0 知识库分组](./knowledge-base-collections-scoped-retrieval.md)
- [V6.0 AI 任务编排](./ai-runtime-and-agent-task-orchestration.md)
- [V7.0 Tool Workflow](./tool-using-agent-workflow.md)
- [V8.0 Ollama 本地运行时](./real-local-ai-runtime-with-ollama.md)
