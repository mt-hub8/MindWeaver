# Personal AI Knowledge Workspace
# 个人 AI 知识工作台




一个**本地优先**的个人 AI 应用：你可以上传自己的资料，配置本地或外部模型，构建个人知识库，并让 AI 基于你的知识执行问答、总结、报告生成和任务编排。

> 仓库技术名仍为 `ai-task-orchestrator`，但产品面向个人用户，**不是企业级 SaaS**。  
> 详细产品说明见 [V9.0 手册](docs/manual/local-personal-knowledge-workspace.md)。



<img width="3824" height="1912" alt="ScreenShot_2026-07-06_140548_155" src="https://github.com/user-attachments/assets/d33fb762-7ba6-4bcd-a409-54e5b570c0ff" />


---

## 1. 项目简介

本项目帮你把「自己的文档」变成「可对话、可检索、可生成报告」的个人知识库。

核心特点：

- **本地优先**：资料与索引留在你的机器或你控制的环境中；配合 Ollama 时，文档内容不必上传到云端大模型服务。
- **个人知识库**：上传 `.txt`、`.md`、文本型 PDF，系统自动切块、向量化并建立索引。
- **多模型可配置**：支持默认 **mock**（开发测试）与 **local-ai**（Ollama 本地模型）；模型信息可在「模型设置」页查看。
- **RAG 问答**（Retrieval-Augmented Generation，检索增强生成）：基于知识库检索后生成回答，并附带**引用来源**，便于核对。
- **AI 任务编排**（AI Task Orchestration）：提交任务后，系统在后台检索知识库、调用工具、生成结构化报告。
- **Tool Workflow**（工具工作流）：固定安全工具链（检索 → 总结 → 报告），可查看每一步执行过程。
- **后续 Skill 扩展**：路线图包含 Skill System，当前阶段尚未实现。

底层具备较完整的 Java 后端工程能力（异步任务、消息队列、向量检索抽象等），但 README 以**产品使用**为主；深入技术细节见文末附录。

---

## 2. 适合谁使用

- **想构建个人知识库的用户** — 论文笔记、项目文档、学习资料统一管理，随时向 AI 提问。
- **想本地体验 RAG 的开发者** — 从上传到检索、引用、问答的完整链路，可在浏览器中操作。
- **想学习 AI 应用后端 / Agent 基础设施的人** — 涵盖文档摄入、Embedding、向量库、异步任务、Tool-Using Workflow 等典型模块。
- **想用本地模型处理私人资料的人** — 通过 Ollama + Python Worker，在本地完成向量化与文本生成，无需把原文发给云端。

---

## 3. 核心功能

- **文档上传** — 支持 `.txt`、`.md`、文本型 PDF，异步处理，不阻塞页面。
- **文本提取** — 从上传文件中提取可索引纯文本（扫描版 PDF / 图片 OCR 暂不支持）。
- **知识库分组** — 按主题把文档归入不同分组，便于分类管理。
- **范围检索** — 问答或 AI 任务时可选择「全部文档」或「仅某一分组」。
- **引用来源** — 回答与报告列出引用的文档片段，降低「AI 胡说」风险。
- **文档生命周期** — ACTIVE / TRASHED / PURGED；删除后进入**垃圾箱**，7 天内可恢复；永久删除后清理底层数据。
- **垃圾箱** — `/trash.html` 查看、恢复或立即永久删除；不参与问答、检索与 AI 任务。
- **本地存储管理** — 系统设置展示存储占用估算与**缓存管理**（清理缓存不删除知识库）。
- **重新索引** — 基于原始文本重新切块与向量化，无需重新上传文件。
- **本地 Ollama 模型** — `local-ai` 模式下使用 `qwen3-embedding` 与 `qwen2.5` 系列模型。
- **模型供应商配置（V10.0）** — 在「模型设置」管理 Ollama / OpenAI-compatible 供应商，设置默认 LLM 与 Embedding；**API Key 不会明文展示**。
- **RAG 质量评分与诊断（V11.0）** — Ask 页面展示**综合评分**、四维质量分、**主要扣分原因**与**优化建议**；支持平衡 / 精准 / 全面三种评分模式；技术详情可折叠查看原始指标与权重。
- **AI 任务编排** — 提交目标后自动生成检索、总结与最终报告。
- **工具执行过程** — 查看每一步工具输入、输出与事件时间线。
- **模型设置** — 查看运行模式、Worker / Ollama 连接状态，支持连接测试。
- **技术详情折叠** — 页面默认中文友好；`metadata`、`traceId` 等工程字段折叠展示，需要时再展开。

---

## 4. 当前产品页面

启动服务后访问 `http://localhost:8080`：

| 页面 | 路径 | 说明 |
|------|------|------|
| 首页 | `/` | 产品入口与核心功能导航 |
| 文档管理 | `/documents.html` | 上传文档、查看状态、重新索引、放入垃圾箱 |
| 垃圾箱 | `/trash.html` | 恢复或永久删除已删除文档 |
| 知识库分组 | `/collections.html` | 创建分组、管理文档归属 |
| 知识库问答 | `/ask.html` | 选择范围提问，查看回答、引用来源与 **RAG 质量评分** |
| AI 任务 | `/agent-tasks.html` | 提交任务，查看报告与执行过程 |
| 模型设置 | `/model-settings.html` | 管理**模型供应商**、默认 LLM/Embedding、**测试连接**（V10.0） |
| 系统设置 | `/settings.html` | 运行模式、本地数据目录规划等只读信息 |
| 文档处理分析 | `/ingestion-analytics.html` | 查看摄入成功率、耗时与失败原因（偏工程向） |

---

## 5. 快速开始

### 前置依赖

开发模式需要：**JDK 17+**、**Maven**（或项目自带 `mvnw.cmd`）、**MySQL**、**RabbitMQ**。  
向量检索推荐 **Qdrant**（见 `docker-compose.qdrant.yml`）。完整环境说明见 [docs/local-dev.md](docs/local-dev.md)。

### 开发模式（默认 mock，不依赖真实模型）

```powershell
cd E:\code\ai-task-orchestrator
docker compose up -d
.\mvnw.cmd test
.\mvnw.cmd spring-boot:run
```

浏览器打开：`http://localhost:8080`

> 默认 **mock** 模式下，Embedding 与 LLM 均为模拟实现，适合跑通流程与自动化测试，**不代表真实模型效果**。

### local-ai 模式（真实本地 Ollama）

1. **启动 Ollama**（桌面应用或 `ollama serve`）
2. **启动 Python AI Runtime Worker**（另开终端）：

```powershell
cd E:\code\ai-task-orchestrator\workers\ai-runtime-worker
pip install -r requirements.txt
python -m uvicorn main:app --host 127.0.0.1 --port 8001
```

3. **启动 Spring Boot（local-ai profile）**：

```powershell
cd E:\code\ai-task-orchestrator
.\mvnw.cmd spring-boot:run -Dspring-boot.run.profiles=local-ai
```

### Windows 一键脚本（可选）

```powershell
cd E:\code\ai-task-orchestrator
.\scripts\windows\check-env.ps1
.\scripts\windows\start-local.ps1
```

说明见 [scripts/windows/README.md](scripts/windows/README.md)。

---

## 6. 本地 Ollama 模型体验

### 安装与拉取模型

1. 安装 [Ollama](https://ollama.com/) 并确保服务运行。
2. 拉取推荐模型：

```powershell
ollama list
ollama pull qwen3-embedding:0.6b
ollama pull qwen2.5:7b
```

内存或显卡较紧张时，LLM 可改用较轻量的 `qwen2.5:3b`：

```powershell
ollama pull qwen2.5:3b
```

### 调用链路说明

```text
Java（Spring Boot）→ Python AI Runtime Worker → Ollama
```

- **Java** 负责业务编排、文档管理、RAG 检索逻辑，**不直接调用 Ollama**。
- **Python Worker** 提供 `/embed` 与 `/generate` 接口，内部调用 Ollama API。
- **Ollama** 在本机运行 Embedding 与 LLM 模型。

启动后可在 **模型设置** 页（`/model-settings.html`）查看连接状态并测试 Embedding / LLM。  
更详细说明：[real-local-ai-runtime-with-ollama.md](docs/manual/real-local-ai-runtime-with-ollama.md)

---

## 7. 使用流程

按以下五步即可走完主路径：

1. **上传文档** — 打开「文档管理」，上传 `.txt` / `.md` / 文本型 PDF。
2. **等待文档处理完成** — 列表状态变为「已完成」后，切块与向量索引才可用于问答。
3. **（可选）创建知识库分组** — 在「知识库分组」中新建分组并加入文档，便于按主题提问。
4. **在知识库问答页面提问** — 选择「全部文档」或指定分组，输入问题，查看回答与引用来源。
5. **在 AI 任务页面生成报告** — 填写任务目标，系统在后台检索、总结并输出结构化报告。

---

## 8. 技术架构

```text
Browser UI（浏览器页面）
    ↓
Spring Boot Backend（Java 业务编排）
    ↓
MySQL / Qdrant / RabbitMQ
    ↓
Python AI Runtime Worker
    ↓
Ollama（本地模型）
```

各组件职责：

| 组件 | 职责 |
|------|------|
| **Java / Spring Boot** | HTTP API、文档生命周期、RAG 编排、AI 任务调度、业务规则 |
| **Python Worker** | AI Runtime：调用 Ollama 完成 Embedding 与文本生成 |
| **Ollama** | 本地运行 Embedding / LLM 模型 |
| **Qdrant** | 向量存储与相似度检索（可配置；亦支持内存 exact 检索 baseline） |
| **RabbitMQ** | 异步任务投递（文档摄入、AI 任务执行等） |
| **MySQL** | 文档、任务、分组、事件等元数据持久化 |

这是典型的 **Java + Python** 分层：Java 做可靠业务底座，Python 做 AI 运行时适配。

---

## 9. 本地数据与隐私

- **当前开发模式**仍使用项目配置中的 MySQL、Qdrant、RabbitMQ 与 Worker 地址，尚未迁移到统一个人数据目录。
- **后续规划**的 Windows 本地数据目录：`%APPDATA%\PersonalAIKnowledgeWorkspace\`（见「系统设置」页说明）。
- **local-ai + Ollama** 模式下，文档原文与向量索引留在本地；向 Ollama 发送的是切块后的文本片段，而非把整个仓库推到云端 SaaS。
- **API Key**：后续将在模型设置中心管理；当前阶段不做完整 Key 加密存储。**请勿把 API Key 提交到 Git**。
- 删除文档进入**垃圾箱（TRASHED）**，**7 天内可恢复**，恢复后无需重新索引。
- **永久删除（PURGED）** 会清理原始文本、片段、向量与相关缓存，不可恢复。
- **缓存管理**可在系统设置中清理 Embedding Cache；**清理缓存不会删除原始文档**。

---

## 10. 默认 mock 与 local-ai profile

| 模式 | 说明 |
|------|------|
| **默认 mock** | 用于开发与自动化测试。Embedding / LLM 均为模拟实现，**不依赖** Ollama、Python Worker、外部 API。`.\mvnw.cmd test` 在此模式下运行。 |
| **local-ai profile** | 启用真实本地链路：Java → Python Worker → Ollama。需手工启动 Worker 与 Ollama，并使用 `qwen3-embedding:0.6b` 等已拉取模型。 |

切换方式：

```powershell
# 开发 / 测试（mock）
.\mvnw.cmd spring-boot:run

# 本地真实模型
.\mvnw.cmd spring-boot:run -Dspring-boot.run.profiles=local-ai
```

配置详见 `application.properties` 与 `application-local-ai.properties`。

### 模型供应商（V10.0）

在 **模型设置** 页面可添加 **Ollama** 与 **OpenAI-compatible** 模型供应商，设置默认问答 / 向量模型。**API Key 不会明文展示**，加密主密钥通过 `app.security.secret-key` 或环境变量 `MODEL_PROVIDER_SECRET_KEY` 配置（勿提交到 Git）。详见 [model-provider-settings.md](docs/manual/model-provider-settings.md)。

---

## 11. 测试

### 默认测试（低依赖）

```powershell
cd E:\code\ai-task-orchestrator
.\mvnw.cmd test
```

**为什么默认测试低依赖？**  
为了保证任何开发者 clone 仓库后，无需安装 Ollama、无需 Docker 中的 Qdrant、无需真实 API Key，也能在 CI 与本地快速验证业务逻辑。测试通过 Maven Surefire 注入 `app.embedding.provider=mock` 与 `app.llm.provider=mock`，并关闭数据库模型覆盖（`app.model-provider.database-overrides-enabled=false`）。**默认测试不依赖真实外部 LLM**，**默认测试不依赖真实外部 API**。

默认测试**不要求**：

- Ollama 已启动  
- Python Worker 已启动  
- Qdrant / Docker 已运行  
- 真实模型已下载  
- 外部网络 / API Key  

### Python Worker 测试（单独运行）

```powershell
cd E:\code\ai-task-orchestrator\workers\ai-runtime-worker
python -m pytest test_worker.py -q
```

Worker 测试使用 mock HTTP，**不调用真实 Ollama**。

---

## 12. 当前不做

请把本项目当作**个人知识工作台原型**，而非可直接上线的企业产品：

- **不是企业 SaaS** — 无多租户、无订阅计费、无运维大屏。
- **不做登录** — 无账号体系。
- **不做多用户** — 单用户本地使用场景。
- **不做权限** — 无角色与访问控制。
- **不做 workspace** — 无团队空间隔离。
- **不做云端同步** — 资料不会自动同步到云端账户。
- **不做多 Agent 圆桌** — 当前为固定工具链，非多 Agent 协作。
- **不做插件市场** — 无第三方插件生态。
- **不做生产级部署** — 无完整监控、限流、高可用方案。
- **不做桌面安装包** — 需自行启动 Java / Python / 依赖服务。

---

## 13. 后续路线

| 版本 | 方向 |
|------|------|
| **V9.0** | 本地个人知识工作台产品化（当前）：中文 UI、模型设置、系统设置、Windows 脚本 |
| **V10.0** | 模型 Provider 设置：Ollama / OpenAI-compatible 供应商、API Key 加密、默认模型 |
| **V11.0** | RAG 质量评分与诊断（当前）：Ask 综合评分、扣分原因、优化建议、三种评分模式 |
| **V12.0** | 垃圾箱与本地存储管理（当前）：ACTIVE → TRASHED → PURGED、7 天恢复、缓存管理 |
| **V13.0** | Batch Ingestion & Notification（批量上传与通知） |
| **V14.0** | Skill System MVP |
| **V15.0** | 本地应用打包（安装包 / 一键服务） |
| **V16.0** | 多 Agent 圆桌 |

---

## 14. 面试表达

若在技术面试中介绍本项目，可从以下角度组织（约 2–3 分钟）：

**产品价值**  
「这是一个本地优先的个人 AI 知识工作台：用户上传私有文档，基于 RAG 获得带引用的问答，并能通过 AI 任务生成结构化报告。」

**系统架构**  
「浏览器 + Spring Boot 做业务编排，MySQL 存元数据，Qdrant 做向量检索，RabbitMQ 做异步任务，Python Worker 适配 Ollama，实现 Java + Python 分层。」

**RAG 工程**  
「文档切块 → Embedding → 向量入库 → TopK 检索 → 引用拼接 → LLM 生成；支持分组范围过滤与文档生命周期过滤，避免已删除或旧版本片段进入上下文。」

**Agent Workflow**  
「AI 任务走固定 Tool Workflow：检索知识库、总结上下文、生成报告；步骤与工具 I/O 可追踪，便于调试与演示。」

**低依赖测试**  
「默认测试全部使用 mock Embedding / LLM，不依赖 Ollama 与外部 API，保证 CI 稳定；真实本地模型通过 `local-ai` profile 与 Worker 集成测试验证。」

更多面试材料：[docs/resume-interview.md](docs/resume-interview.md)

---

## 15. FAQ

### 为什么不叫「企业级」？

因为产品定位是**个人本地知识工作台**，不是面向多租户、权限、计费的企业 SaaS。底层虽有较完整的任务编排与 RAG 工程，但用户体验与边界按单用户场景收敛。

### 为什么 Java + Python？

Java（Spring Boot）擅长可靠的业务服务、事务、异步任务与 API 治理；Python 更适合快速对接 Ollama 与 AI Runtime。两者通过 HTTP 解耦，各自演进。

### 为什么默认不用真实模型？

为了让 `.\mvnw.cmd test` 与 CI **零外部依赖**、结果稳定。真实效果请在 `local-ai` profile 下体验。

### Ollama 失败怎么办？

1. 确认 Ollama 已启动：`ollama list`  
2. 确认模型已拉取：`ollama pull qwen3-embedding:0.6b`  
3. 确认 Python Worker 运行且 `http://127.0.0.1:8001/health` 可访问  
4. 在「模型设置」页执行连接测试  
5. 详见 [scripts/windows/README.md](scripts/windows/README.md) 与 [Ollama 手册](docs/manual/real-local-ai-runtime-with-ollama.md)

### Embedding 维度变了怎么办？

向量维度必须与索引一致。若从 mock（128 维）切换到 Ollama（如 1024 维），需**删除旧文档并重新上传**，或重新建立索引，否则检索会异常。

### 为什么删除文档后不立即物理删除向量？

采用**垃圾箱**策略：删除后进入 TRASHED，7 天内可恢复且保留 chunks / vectors；到期或手动 **PURGED** 后清理底层数据。详见 [垃圾箱与本地存储管理](docs/manual/trash-and-local-storage-management.md)。

### 上千文档后续怎么处理？

当前原型面向个人规模（数百篇量级）。更大规模需：Qdrant 集群调优、分批摄入、检索策略优化、可能的冷热分层——列入后续路线图，**当前不做**生产级海量优化承诺。

---

## 附录：开发文档索引

主 README 以产品说明为主。按能力查阅详细手册：

| 主题 | 文档 |
|------|------|
| V9.0 个人知识工作台 | [local-personal-knowledge-workspace.md](docs/manual/local-personal-knowledge-workspace.md) |
| V10.0 模型供应商设置 | [model-provider-settings.md](docs/manual/model-provider-settings.md) |
| 上传与问答 | [upload-to-ask.md](docs/manual/upload-to-ask.md) |
| Knowledge Base Lifecycle（V4.0）· 软删除 · 重新索引 | [knowledge-base-lifecycle-management.md](docs/manual/knowledge-base-lifecycle-management.md) |
| 知识库分组 · 范围检索（V5.0）· `collectionId` | [scoped-retrieval-and-collections.md](docs/manual/scoped-retrieval-and-collections.md) |
| AI 任务编排（V6.0） | [ai-runtime-and-agent-task-orchestration.md](docs/manual/ai-runtime-and-agent-task-orchestration.md) |
| Tool Workflow（V7.0）· `/agent-tasks.html` | [tool-using-agent-workflow.md](docs/manual/tool-using-agent-workflow.md) |
| 本地 Ollama（V8.0）· `real-local-ai-runtime-with-ollama.md` | [docs/manual/real-local-ai-runtime-with-ollama.md](docs/manual/real-local-ai-runtime-with-ollama.md) |
| 本地开发环境 | [docs/local-dev.md](docs/local-dev.md) |
| API 示例 | [docs/api-examples.md](docs/api-examples.md) |
| 面试 deep-dive | [docs/interview](docs/interview) |

---

**Personal AI Knowledge Workspace** — 你的文档，你的模型，你的知识库。
