# 本地开发环境

本文说明 `ai-task-orchestrator` 在 Windows 本地如何使用 Docker Compose 启动 MySQL / RabbitMQ，如何启动 Spring Boot，以及如何手工验证 Embedding / VectorStore / Worker 等实验性能力。

## 一、前置要求

- JDK 21+
- Docker Desktop
- Maven Wrapper（项目自带 `mvnw.cmd`）
- PowerShell

进入项目目录：

```powershell
cd E:\code\ai-task-orchestrator
```

不要使用：

```powershell
cd /d E:\code\ai-task-orchestrator
```

Spring Boot 在本机启动，不放进 Docker 容器（MySQL / RabbitMQ 除外）。

---

## 二、启动 MySQL / RabbitMQ

```powershell
docker compose up -d
docker compose ps
```

| 服务 | 本机端口 |
| --- | --- |
| MySQL | `3307` → 容器 `3306` |
| RabbitMQ AMQP | `5672` |
| RabbitMQ 管理台 | `15672`（guest / guest） |

`docker` profile 连接：

- MySQL: `localhost:3307/ai_task_orchestrator`
- RabbitMQ: `localhost:5672`

停止：

```powershell
docker compose down
```

---

## 三、启动 Spring Boot

```powershell
.\mvnw.cmd spring-boot:run "-Dspring-boot.run.profiles=docker"
```

IDEA 中设置 Active profiles: `docker`。

配置读取 `src/main/resources/application-docker.properties`（JDBC 端口 3307）。

---

## 四、默认 Provider 配置

`application.properties` 默认值（本地非 docker profile 亦类似）：

```properties
app.embedding.provider=mock
app.vector-store.provider=exact
```

含义：

- 文本向量由 Mock Embedding 生成
- 向量检索由 `ExactCosineVectorStore` 完成（复用 `document_chunk_embedding`）

无需 OpenAI API key、Python worker 或 Qdrant 即可跑通 document search / retrieval evaluation / RAG answer（Mock LLM）。

---

## 五、OpenAI-compatible Embedding（手工，非默认测试）

```properties
app.embedding.provider=openai
app.embedding.openai.api-key=${OPENAI_API_KEY}
app.embedding.openai.model=text-embedding-3-small
app.embedding.openai.dimension=1536
```

需有效 API key 与外部网络。默认 `.\mvnw.cmd test` 不调用 OpenAI。

---

## 六、Local Embedding Worker（手工，非默认测试）

Worker 路径：`workers/embedding-worker/`

```powershell
cd E:\code\ai-task-orchestrator\workers\embedding-worker
python -m venv .venv
.\.venv\Scripts\Activate.ps1
pip install -r requirements.txt
uvicorn main:app --host 127.0.0.1 --port 8001
```

Java 侧配置：

```properties
app.embedding.provider=local-worker
app.embedding.local-worker.base-url=http://127.0.0.1:8001
app.embedding.local-worker.model=sentence-transformers/all-MiniLM-L6-v2
app.embedding.local-worker.dimension=384
```

首次运行会下载或加载模型，耗时不定。默认 Maven test 不启动 Python worker。

---

## 七、Qdrant（手工，非默认测试）

启动 Qdrant（示例，需本机 Docker）：

```powershell
cd E:\code\ai-task-orchestrator
docker run -p 6333:6333 -p 6334:6334 -v ${PWD}\qdrant_storage:/qdrant/storage qdrant/qdrant
```

Java 侧配置：

```properties
app.vector-store.provider=qdrant
app.vector-store.qdrant.base-url=http://127.0.0.1:6333
app.vector-store.qdrant.collection-name=ai_task_orchestrator_chunks
app.vector-store.qdrant.initialize-collection=true
```

说明：

- Qdrant 未纳入项目 `docker-compose.yml`
- 默认测试使用 fake client，不连接真实 Qdrant
- 这是实验性手工验证路径，不是 production deployment

可与 local-worker 组合：local-worker 生成向量 + Qdrant 存储检索。

---

## 八、运行测试

```powershell
cd E:\code\ai-task-orchestrator
.\mvnw.cmd test
```

默认测试保证：

- 不启动 Docker（本地可选）
- 不连接 Qdrant
- 不访问 OpenAI API
- 不启动 Python embedding worker
- 不下载 embedding 模型
- 不访问外部网络

测试数量以命令输出为准。

---

## 九、dev profile 调试接口

启用 `dev` profile 后可使用：

```http
PATCH http://localhost:8080/dev/tasks/{taskId}/status
```

用于状态机调试，不是生产 API。

---

## 十、常见问题

### 端口冲突

MySQL 映射 `3307` 是为避免与本机 `3306` 冲突。若仍冲突，修改 `docker-compose.yml` 与 `application-docker.properties` 保持一致。

### MySQL 连接失败

1. `docker compose ps` 确认容器 healthy
2. 确认使用 `docker` profile
3. JDBC 端口应为 `3307`（docker profile）

### RabbitMQ 连接失败

检查 `5672` / `15672` 是否被占用：`netstat -ano | findstr :5672`

### Flyway 校验失败

```powershell
docker compose down -v
docker compose up -d
```

`-v` 会删除 Compose 数据卷，本地数据清空。

### Embedding / Search 无结果

1. 确认已 `POST /documents/{documentId}/embeddings`
2. search 使用的 provider/model 与 embedding 时一致（默认均为 mock）
3. 切换 provider 后需重新 embed

### 切换 Qdrant 后检索异常

1. 确认 Qdrant 容器运行中
2. `initialize-collection=true` 且 dimension 与 embedding 一致
3. 同一 collection 不应混用不同 embedding space

---

## 相关文档

- [README.md](../README.md)
- [docs/api-examples.md](api-examples.md)
- [docs/project-structure.md](project-structure.md)
