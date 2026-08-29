# V10.0 模型供应商设置（Model Provider Settings）

## 目标

将「模型设置」从只读展示升级为**可配置的模型供应商中心**：用户可在页面中管理 Ollama、OpenAI-compatible 等供应商，设置默认 LLM / Embedding，并测试连接。

## 为什么要做

- V9.0 仅展示运行时状态，无法保存用户配置；
- 个人知识工作台需要支持**本地 Ollama** 与**外部 OpenAI-compatible API**（DeepSeek、Qwen、SiliconFlow 等）；
- API Key 不能明文出现在前端或日志中；
- Ask / AI 任务应使用用户选择的默认模型，而不是写死在代码里。

## 支持的 Provider 类型

| 类型 | 说明 |
|------|------|
| `MOCK` | 开发测试，内置 Mock，不调用真实模型 |
| `OLLAMA` | 本地 Ollama（连接测试访问 `/api/tags`；运行时经 Python Worker） |
| `OPENAI_COMPATIBLE` | 预设模板（DeepSeek / Qwen / SiliconFlow） |
| `CUSTOM_OPENAI_COMPATIBLE` | 用户自定义 Base URL 与模型 |

**本阶段不做** Gemini / Claude 等非 OpenAI-compatible 协议。

## Ollama 本地配置

1. 在「模型设置」选择预设 **本地 Ollama** 或手动创建 `OLLAMA` 类型；
2. Base URL 默认 `http://127.0.0.1:11434`；
3. 填写默认 LLM（如 `qwen2.5:7b`）与 Embedding（如 `qwen3-embedding:0.6b`）；
4. **不需要 API Key**；
5. 点击「测试连接」验证 Ollama 可达；
6. 「设为默认问答模型」/「设为默认向量模型」。

运行时链路仍为：**Java → Python Worker → Ollama**（Java 不直接调 Ollama 生成）。

## OpenAI-compatible 配置

1. 选择预设 **DeepSeek / Qwen / SiliconFlow** 或「自定义 OpenAI-compatible」；
2. 填写 Base URL、API Key、默认 LLM / Embedding 模型；
3. 保存后仅显示 **masked Key**（如 `sk-****abcd`）；
4. 用户主动点击「测试连接」时，才会请求 `/v1/models`（默认自动化测试不调用）。

## API Key 如何保存

- 前端提交 `apiKey` 字段；
- 后端使用 **AES-GCM** 加密存入 `api_key_encrypted`；
- 主密钥来自 `app.security.secret-key` 或环境变量 `MODEL_PROVIDER_SECRET_KEY`；
- **GET 响应只返回 `apiKeyMasked`**，永不返回明文；
- 更新时 `apiKey` 为空则**保留原 Key**；
- 日志不打印明文 Key。

## 默认 LLM / Embedding

- 全局仅允许**一个** `default_llm` 与一个 `default_embedding`；
- 已禁用的供应商**不能**设为默认；
- 若数据库未配置默认，或测试环境关闭 `app.model-provider.database-overrides-enabled`，则回退到 `application.properties`（测试为 mock）。

## 切换 Embedding 与重新索引

向量维度必须与已有索引一致。切换 Embedding 模型后，页面会提示可能需要**重新索引**；V10.0 不会自动重建索引。

## 默认 mock 与真实 Provider

| 场景 | 行为 |
|------|------|
| `.\mvnw.cmd test` | mock + 不启用 DB 覆盖 + 不调用外部 API |
| 设置 DB 默认供应商 | 启用 `database-overrides-enabled` 后使用 DB 配置 |

## 当前边界

- 不做登录 / 多用户 / 计费 / quota；
- 不做 Gemini / Claude 特殊协议；
- 不做模型市场。

## API 一览

见 `ModelProviderController`：`/model-providers` 系列端点。
