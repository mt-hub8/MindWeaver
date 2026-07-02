# V8.0 真实本地 AI Runtime（Ollama）

## 1. 本阶段目标

V8.0 让你在 **Windows 本地免费环境** 中体验真实 embedding 与真实 LLM，架构如下：

```text
Java Spring Boot
  -> LocalEmbeddingWorkerProvider / LocalPythonLlmProvider
  -> Python AI Runtime Worker (port 8001)
  -> Ollama (port 11434)
```

- **默认 profile** 仍使用 **mock** embedding / mock LLM
- **local-ai profile** 才启用真实模型
- **默认 `.\mvnw.cmd test`** 不依赖 Ollama、不启动 Python Worker、不下载模型

默认模型：

| 用途 | 模型 | 说明 |
|------|------|------|
| Embedding | `qwen3-embedding:0.6b` | 本地向量 |
| LLM（推荐） | `qwen2.5:7b` | 质量较好，8GB 显存可能偏慢 |
| LLM（备用） | `qwen2.5:3b` | CPU / 低显存更快 |

---

## 2. Windows 配置检查

```powershell
nvidia-smi
systeminfo | findstr /C:"Total Physical Memory" /C:"Available Physical Memory"
```

建议：32GB 内存 + 8~12GB 显存可运行 7B；若太慢请切换 3B。

---

## 3. 安装 Ollama

1. 从 [https://ollama.com](https://ollama.com) 下载 Windows 安装包
2. 安装后验证：

```powershell
ollama --version
```

确保 Ollama 服务在 `http://localhost:11434` 可访问。

---

## 4. 拉取模型

```powershell
ollama pull qwen3-embedding:0.6b
ollama pull qwen2.5:7b
```

若 7B 太慢，改用低配置备用模型：

```powershell
ollama pull qwen2.5:3b
```

若 `qwen3-embedding:0.6b` 拉取失败，可尝试：

```powershell
ollama pull qwen3-embedding
```

拉取后设置环境变量（可选）：

```powershell
$env:OLLAMA_LLM_MODEL="qwen2.5:3b"
```

---

## 5. 测试 Ollama Embedding

```powershell
curl http://localhost:11434/api/embed -Method POST -Body '{"model":"qwen3-embedding:0.6b","input":"这是一个知识库测试文本"}' -ContentType "application/json"
```

应返回 `embeddings` 数组。

---

## 6. 测试 Ollama LLM

```powershell
curl http://localhost:11434/api/generate -Method POST -Body '{"model":"qwen2.5:7b","prompt":"用中文解释 RAG 是什么","stream":false}' -ContentType "application/json"
```

若太慢：

```powershell
curl http://localhost:11434/api/generate -Method POST -Body '{"model":"qwen2.5:3b","prompt":"用中文解释 RAG 是什么","stream":false}' -ContentType "application/json"
```

---

## 7. 启动 Python Worker

```powershell
cd E:\code\ai-task-orchestrator\workers\ai-runtime-worker
python -m venv .venv
.\.venv\Scripts\activate
pip install -r requirements.txt
uvicorn main:app --host 0.0.0.0 --port 8001
```

验证：

```powershell
curl http://127.0.0.1:8001/health
```

应看到 `provider: local-ollama`、`embeddingModel`、`llmModel`、`ollamaReachable`。

环境变量（可选）：

| 变量 | 默认值 |
|------|--------|
| `OLLAMA_BASE_URL` | `http://localhost:11434` |
| `OLLAMA_EMBEDDING_MODEL` | `qwen3-embedding:0.6b` |
| `OLLAMA_LLM_MODEL` | `qwen2.5:7b` |
| `OLLAMA_TIMEOUT_SECONDS` | `120` |

---

## 8. 启动 Spring Boot（local-ai profile）

新开终端：

```powershell
cd E:\code\ai-task-orchestrator
.\mvnw.cmd spring-boot:run "-Dspring-boot.run.profiles=local-ai"
```

此时：

- `app.embedding.provider=local-worker` → Python `/embeddings` → Ollama `/api/embed`
- `app.llm.provider=local-python` → Python `/generate` → Ollama `/api/generate`

---

## 9. 端到端体验路径

1. 打开 **文档管理** `/documents.html`，上传 `.md` / `.txt` / 文本型 PDF
2. 等待 **异步 ingestion** 完成（生成真实 embedding 并写入向量库）
3. 打开 **知识库问答** `/ask.html`，提问，查看 **citations** 与回答
4. 打开 **AI 任务编排** `/agent-tasks.html`，创建 Agent Task
5. 查看 **任务执行计划**、**工具执行记录**（检索知识库 → 总结检索结果 → 生成最终报告）
6. 查看 **最终报告** 与 **引用来源**

可选：访问 `/agent-tools.html` 查看可用内置工具。

---

## 10. 常见问题

### Ollama 未启动

- 症状：Python `/health` 显示 `ollamaReachable: false`；调用失败
- 处理：启动 Ollama 应用，确认 `http://localhost:11434` 可访问

### 模型未 pull

- 症状：Ollama 返回 model not found
- 处理：执行 `ollama pull <model-name>`

### CPU 很慢

- 使用 `qwen2.5:3b` 作为 LLM
- 设置 `$env:OLLAMA_LLM_MODEL="qwen2.5:3b"` 并重启 Python Worker

### qwen2.5:7b 太慢

- 改用 3B，或在 `application-local-ai.properties` 中修改 `app.llm.model=qwen2.5:3b`

### Embedding 维度变化导致旧向量不兼容

- 从 mock（384 维）或旧模型切换到 `qwen3-embedding:0.6b`（通常 1024 维）后，**必须重新上传 / 重新索引文档**
- 清空或重建向量数据后再 ingestion

### 为什么默认 test 仍然 mock？

- 保证 CI 与本地 `mvn test` 快速、稳定、无 GPU / 无网络依赖

### 为什么不要把真实模型接入默认 profile？

- 避免未安装 Ollama 时应用无法启动或测试失败

### 如何切回 mock？

- 不使用 `local-ai` profile，直接默认启动即可
- 或显式：`.\mvnw.cmd spring-boot:run`（不带 profile）

---

## 11. 当前不做

- 不接云端 API（OpenAI / Gemini / Groq）
- 不做 streaming / WebSocket / SSE
- 不做 vLLM / SGLang
- 不做生产部署与 GPU 优化
- Java **不直接**调用 Ollama，必须经过 Python Worker

---

## 相关文件

- Python Worker：`workers/ai-runtime-worker/`
- Java profile：`src/main/resources/application-local-ai.properties`
- V7 Tool Workflow 说明：[tool-using-agent-workflow.md](./tool-using-agent-workflow.md)
