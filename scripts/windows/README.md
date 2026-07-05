# Windows 本地启动说明

本目录提供 **Personal AI Knowledge Workspace（个人 AI 知识工作台）** 在 Windows 上的基础环境检查与一键启动脚本。

## 前置条件

- JDK 17+
- Maven（或使用项目自带的 `mvnw.cmd`）
- Python 3.10+（运行 `workers/ai-runtime-worker`）
- Ollama（local-ai 模式）
- MySQL、RabbitMQ、Qdrant（按项目 README 与 docker-compose 启动）

推荐模型（local-ai）：

```powershell
ollama pull qwen3-embedding:0.6b
ollama pull qwen2.5:7b
```

也可使用 `qwen2.5:3b` 作为较轻量的 LLM。

## 如何检查环境

在项目根目录执行：

```powershell
.\scripts\windows\check-env.ps1
```

脚本会检查 Java、Python、Ollama、推荐模型，以及端口 **8001**（Worker）、**8080**（Spring Boot）、**11434**（Ollama）。

## 如何启动

1. 确保 MySQL、RabbitMQ、Qdrant 等依赖服务已运行。
2. 启动 Ollama（应用或 `ollama serve`）。
3. 在项目根目录执行：

```powershell
.\scripts\windows\start-local.ps1
```

脚本会：

- 在新窗口启动 Python Worker（8001）
- 在新窗口以 `local-ai` profile 启动 Spring Boot（8080）
- 约 30 秒后打开浏览器 `http://localhost:8080`

## 如何停止

```powershell
.\scripts\windows\stop-local.ps1
```

若进程是手动在独立窗口启动的，请直接关闭对应窗口。本脚本**不会**强杀所有 Java/Python/Ollama 进程。

## 常见问题

### 端口被占用怎么办？

- **8001 已占用**：可能已有 Worker 在运行；关闭旧窗口，或修改 `application-local-ai.properties` 中的 Worker 地址（需与 Worker 端口一致）。
- **8080 已占用**：关闭占用 8080 的其他应用，或修改 `server.port`。
- **11434 不可访问**：启动 Ollama 桌面应用，或命令行运行 `ollama serve`。

使用 `check-env.ps1` 可快速查看端口状态。

### 模型没下载怎么办？

```powershell
ollama pull qwen3-embedding:0.6b
ollama pull qwen2.5:7b
```

然后在浏览器打开 **模型设置**（`/model-settings.html`）进行连接测试。

### 页面显示 mock 模型？

默认 profile 使用 mock 便于开发。本地真实模型请使用：

```powershell
.\mvnw.cmd spring-boot:run -Dspring-boot.run.profiles=local-ai
```

或运行 `start-local.ps1`（已带 local-ai profile）。

### 测试失败但 Ollama 正常？

1. 确认 Python Worker 窗口无报错。
2. 访问 `http://127.0.0.1:8001/health` 查看 Worker 与 Ollama 状态。
3. 旧文档若使用 mock 向量索引，需删除后重新上传。

## 相关文档

- [V9.0 个人知识工作台手册](../../docs/manual/local-personal-knowledge-workspace.md)
- [V8.0 Ollama 本地运行时](../../docs/manual/real-local-ai-runtime-with-ollama.md)
- [项目 README](../../README.md)
