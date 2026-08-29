# Local Embedding Worker

AI Task Orchestrator 的 **本地 Embedding Provider 原型 / 工程组件**。通过 FastAPI 暴露 HTTP API，使用 [sentence-transformers](https://www.sbert.net/) 在本地生成文本向量，供 Java 侧 `LocalEmbeddingWorkerProvider` 调用。

**本 worker 不是生产级 embedding serving 平台**，不提供多租户、自动扩缩容、模型热切换注册中心或 SLA 保障。适用于本地验证、开发与手工联调。

---

## 1. Worker 是什么

| 项 | 说明 |
| --- | --- |
| 技术栈 | Python 3 + FastAPI + uvicorn + sentence-transformers |
| 入口 | `main.py`（`app` 实例） |
| 默认端口 | `8001`（与 Java `app.embedding.local-worker.base-url` 默认一致） |
| Provider 名称 | `local-worker`（与 Java `LocalEmbeddingWorkerProvider.PROVIDER` 一致） |
| 默认模型 | `sentence-transformers/all-MiniLM-L6-v2`（dimension **384**） |

Java 通过 `POST /embeddings` 获取向量，再写入 `VectorStore`（exact 或 Qdrant）。**embedding 的正确性由 provider / model / dimension 共同决定**，不是单纯“能连上就行”。

---

## 2. 为什么需要 Local Embedding Worker

- 在**不调用 OpenAI API** 的前提下，使用真实语义向量（非 mock）验证 RAG 链路
- 与 Java `EmbeddingProvider` 抽象对齐：`app.embedding.provider=local-worker`
- 本地可复现、可排障，便于 reviewer 理解协议与配置边界

默认 `.\mvnw.cmd test` **不会**启动本 worker、**不会**下载模型、**不依赖**外部网络。

---

## 3. 前置条件

- Python **3.10+**（建议 3.11 或 3.12）
- 能访问 PyPI（`pip install`）
- 首次使用需能访问 Hugging Face / 模型仓库（下载 `all-MiniLM-L6-v2`）
- 磁盘空间：模型缓存约数百 MB（视环境而定）

---

## 4. 创建虚拟环境并安装依赖

PowerShell：

```powershell
cd E:\code\ai-task-orchestrator\workers\embedding-worker

python -m venv .venv
.\.venv\Scripts\Activate.ps1

pip install -r requirements.txt
```

`requirements.txt` 已锁定**直接依赖**版本（传递依赖由 pip 解析，本目录未提供完整 lock file）：

| 包 | 版本 |
| --- | --- |
| fastapi | 0.138.2 |
| uvicorn | 0.49.0 |
| sentence-transformers | 5.6.0 |

---

## 5. 启动 Worker

```powershell
cd E:\code\ai-task-orchestrator\workers\embedding-worker
.\.venv\Scripts\Activate.ps1

uvicorn main:app --host 0.0.0.0 --port 8001
```

- 入口文件：`main.py`
- ASGI app 变量名：`app`
- 监听：`http://0.0.0.0:8001`

启动后勿关闭该终端；另开终端启动 Spring Boot。

---

## 6. 默认模型与首次下载

- 默认模型：`sentence-transformers/all-MiniLM-L6-v2`
- 输出维度：**384**
- 首次请求某 `model` 时，`sentence-transformers` 会从 Hugging Face **下载并缓存**到本机（常见路径：`~/.cache/huggingface/` 或 Windows 用户目录下等价路径）
- **首次启动 / 首次 embed 可能较慢**（数分钟，取决于网络与磁盘）
- 无网络环境需提前准备模型缓存，或在内网镜像源配置 Hugging Face 访问
- 默认 Maven test **不会**触发下载

Worker 使用 `@lru_cache` 按 `model` 名称缓存已加载模型（进程内最多 4 个）。

---

## 7. CPU / GPU 边界

| 模式 | 说明 |
| --- | --- |
| **CPU（默认建议）** | 本地验证推荐；小模型 `all-MiniLM-L6-v2` 可在 CPU 运行；批量或长文本时延迟明显 |
| **GPU** | 需本机安装匹配的 CUDA 与 PyTorch；**当前项目不保证 GPU 开箱即用**；本版本不做 GPU 自动检测与优化 |

不要假设：

- 自动支持所有 GPU
- 生产级高性能 embedding serving
- 相对 mock / 其他 provider 的“性能显著提升”（未在本仓库做 benchmark 承诺）

---

## 8. Health Check

```http
GET http://127.0.0.1:8001/health
```

响应示例（不加载模型、不返回 dimension）：

```json
{
  "status": "ok",
  "provider": "local-worker"
}
```

PowerShell：

```powershell
Invoke-WebRequest -Uri http://127.0.0.1:8001/health -UseBasicParsing
```

---

## 9. Embed API 协议

### 端点

```http
POST http://127.0.0.1:8001/embeddings
Content-Type: application/json
```

> 注意：路径为 **`/embeddings`**，请求字段为 **`model`** 与 **`input`**（不是 `/embed` 或 `texts`）。

### 请求体

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 是 | Hugging Face / sentence-transformers 模型名 |
| `input` | string 或 string[] | 是 | 单条文本或文本列表 |

单条示例：

```json
{
  "model": "sentence-transformers/all-MiniLM-L6-v2",
  "input": "AI Task Orchestrator is a backend system."
}
```

批量示例：

```json
{
  "model": "sentence-transformers/all-MiniLM-L6-v2",
  "input": [
    "AI Task Orchestrator is a backend system.",
    "Vector search needs stable embedding dimensions."
  ]
}
```

### 响应体

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `provider` | string | 固定 `local-worker` |
| `model` | string | 与请求一致 |
| `dimension` | int | 向量维度（`all-MiniLM-L6-v2` 为 384） |
| `data` | array | 每项含 `index`、`embedding`（float 数组） |

响应示例：

```json
{
  "provider": "local-worker",
  "model": "sentence-transformers/all-MiniLM-L6-v2",
  "dimension": 384,
  "data": [
    {
      "index": 0,
      "embedding": [0.012, -0.034, "..."]
    }
  ]
}
```

错误时返回 HTTP 4xx/5xx，body 为 FastAPI 默认 `{"detail": "..."}` 格式。

更多请求见同目录 [`embedding-worker.http`](embedding-worker.http)。

---

## 10. Java Spring Boot 对接

### 配置项（`application.properties` 或 profile 覆盖）

字段名以 `EmbeddingProperties` / `application.properties` 为准：

```properties
app.embedding.provider=local-worker
app.embedding.local-worker.base-url=http://127.0.0.1:8001
app.embedding.local-worker.model=sentence-transformers/all-MiniLM-L6-v2
app.embedding.local-worker.dimension=384
app.embedding.local-worker.timeout-ms=10000
```

Java 客户端：`RestClientLocalEmbeddingWorkerClient` → `POST {base-url}/embeddings`。

### 手工联调步骤

1. 启动本 worker（上一节）
2. 启动 Spring Boot（需 MySQL / RabbitMQ，见项目 `docs/local-dev.md`）
3. 设置 `app.embedding.provider=local-worker`（及上述 local-worker 项）
4. 调用 `POST /documents/{documentId}/embeddings` → `POST /documents/search`

若使用 Qdrant，需保证 collection dimension 与 **384** 一致，且 collection 内未混入其他 model 的向量。

---

## 11. Provider / Model / Dimension 正确性边界

向量空间由 **provider + model + dimension** 共同定义：

1. **不同 provider**（mock / openai / local-worker）的向量**不可**在同一检索空间直接比较  
2. **不同 model** 即使 dimension 相同，语义空间也不同，**换模型后应重新 embed**  
3. **dimension 不一致**会导致 Java 校验失败、Qdrant upsert 失败或检索结果无意义  
4. `VectorStore` / Qdrant payload 会记录 `provider`、`model`、`dimension` metadata，用于过滤——**不能混用**

示例：

- 先用 `mock`（128 维）写入 Qdrant，再切 `local-worker`（384 维）→ **必须**新 collection 或清空后重建  
- Java 配置 `dimension=384` 但 worker 实际返回 768 → `LocalEmbeddingWorkerProvider` 校验失败  

这是**检索正确性边界**，不是“改个配置项就行”。

---

## 12. 常见问题

### 端口 8001 被占用

```powershell
netstat -ano | findstr :8001
```

更换端口时，同时修改 `uvicorn ... --port` 与 `app.embedding.local-worker.base-url`。

### Python 版本不兼容

建议使用 Python 3.10+。`pip install` 报错时检查 `python --version`。

### pip 安装失败

- 升级 pip：`python -m pip install --upgrade pip`
- 使用国内 PyPI 镜像（如清华源）仅作排障，团队环境自行评估

### 首次模型下载慢 / 失败

- 确认可访问 Hugging Face
- 查看 worker 终端日志中的 `failed to load embedding model`
- 配置 `HF_ENDPOINT` 或离线缓存（高级排障，非本仓库默认范围）

### Java 连接 worker 失败

- 确认 worker 已启动且 `GET /health` 返回 200
- 确认 `base-url` 与 worker 端口一致（默认 `http://127.0.0.1:8001`）
- 确认 `timeout-ms` 足够（首次 embed 含模型加载可能超过默认 10s，可适当调大）

### dimension 与 Java 配置不一致

- `all-MiniLM-L6-v2` 应为 **384**
- 修改 `app.embedding.local-worker.dimension` 必须与 worker 实际返回一致

### Qdrant collection 混入不同模型向量

- 换 model 后重新 `POST /documents/{id}/embeddings`
- 或删除 collection / 使用新 `collection-name`

### CPU 推理慢

- 正常现象；减小 batch、缩短文本，或仅用于验证而非压测

---

## 13. 本 Worker 不是什么

- 不是生产级 embedding 服务平台  
- 不是多模型动态路由 / worker 注册中心  
- 不包含 Dockerfile / Kubernetes 部署（本版本不做）  
- 不包含 GPU 自动化与性能 benchmark  
- 默认 CI / `mvn test` 不依赖本进程  

---

## 14. 相关代码

| 位置 | 说明 |
| --- | --- |
| `workers/embedding-worker/main.py` | FastAPI 入口 |
| `src/main/java/.../embedding/LocalEmbeddingWorkerProvider.java` | Java Provider |
| `src/main/java/.../embedding/RestClientLocalEmbeddingWorkerClient.java` | HTTP 客户端 |
| `src/main/java/.../embedding/EmbeddingProperties.java` | 配置属性 |

项目级说明见 `docs/local-dev.md`（未修改根 README）。
