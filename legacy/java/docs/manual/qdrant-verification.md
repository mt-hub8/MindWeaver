# Qdrant 手工验证指南

本文说明如何在本地启动**真实 Qdrant**，并通过 `qdrant` Spring profile 验证当前 `QdrantVectorStore` 集成。

本指南面向 **手工验证 / reviewer**，不是 production deployment，也不改变默认 `.\mvnw.cmd test` 行为（默认测试仍使用 fake client，不连接 Qdrant）。

---

## 1. 验证目标

| 步骤 | 说明 |
| --- | --- |
| Qdrant 启动 | `docker-compose.qdrant.yml` 独立启动 Qdrant |
| Profile 生效 | `application-qdrant.properties` 将 `app.vector-store.provider` 设为 `qdrant` |
| Collection 初始化 | `initialize-collection=true` 时首次 upsert 自动创建 collection |
| 端到端链路 | 文档上传 → chunk → mock embedding → Qdrant upsert → search |
| 默认测试隔离 | `mvn test` 不依赖 Qdrant / Docker |

---

## 2. 前置条件

- Docker Desktop
- JDK 21+
- 项目主基础设施已启动（MySQL + RabbitMQ），见 [docs/local-dev.md](../local-dev.md)

进入项目目录：

```powershell
cd E:\code\ai-task-orchestrator
```

---

## 3. 启动 Qdrant（独立 Compose）

不修改主 `docker-compose.yml`，使用专用文件：

```powershell
cd E:\code\ai-task-orchestrator
docker compose -f docker-compose.qdrant.yml up -d
docker compose -f docker-compose.qdrant.yml ps
```

| 项 | 值 |
| --- | --- |
| Image | `qdrant/qdrant:v1.12.5` |
| HTTP API | `http://127.0.0.1:6333` |
| gRPC | `6334` |
| 数据卷 | `qdrant-data`（命名卷） |

停止：

```powershell
docker compose -f docker-compose.qdrant.yml down
```

清空数据卷（可选，会删除 collection 数据）：

```powershell
docker compose -f docker-compose.qdrant.yml down -v
```

---

## 4. 健康检查

PowerShell：

```powershell
Invoke-WebRequest -Uri http://127.0.0.1:6333/healthz -UseBasicParsing
```

期望：HTTP 200，响应体含 `healthz check passed` 或类似内容。

查看 collections（验证前可能为空）：

```powershell
Invoke-WebRequest -Uri http://127.0.0.1:6333/collections -UseBasicParsing
```

---

## 5. 启动 Spring Boot（docker + qdrant profile）

先确保主 Compose 已运行：

```powershell
docker compose up -d
```

再启动应用（**同时启用 `docker` 与 `qdrant` profile**）：

```powershell
cd E:\code\ai-task-orchestrator
.\mvnw.cmd spring-boot:run "-Dspring-boot.run.profiles=docker,qdrant"
```

`application-qdrant.properties` 关键配置（与代码中 `VectorStoreProperties` 一致）：

```properties
app.vector-store.provider=qdrant
app.vector-store.qdrant.base-url=http://127.0.0.1:6333
app.vector-store.qdrant.collection-name=ai_task_orchestrator_chunks
app.vector-store.qdrant.initialize-collection=true
app.vector-store.qdrant.timeout-ms=10000
```

默认 **embedding 仍为 mock**（`app.embedding.provider=mock`，dimension **128**）。不要在同一 collection 中混用不同 embedding provider / dimension。

---

## 6. HTTP 手工验证

推荐使用 `docs/demo/qdrant-flow.http`（IDE REST Client 逐步执行）。

最小 curl 等价流程：

1. `POST /documents` — 上传 `.md` 文件  
2. `GET /documents/{documentId}/chunks` — 确认 chunks  
3. `POST /documents/{documentId}/embeddings` — mock 向量写入 Qdrant  
4. `POST /documents/search` — 经 `QdrantVectorStore` 检索  

示例 search 请求体：

```json
{
  "query": "verify Qdrant upsert and vector search",
  "topK": 5,
  "documentId": 1
}
```

期望：

- embedding 响应中 `embeddingProvider=mock`，`dimension=128`
- search 返回 `chunkId`、`score`、`content` 等字段
- 无 `VECTOR_STORE_ERROR` 类 API 错误

---

## 7. 验证 Qdrant 侧数据

首次 embed 成功后，检查 collection 是否存在：

```powershell
Invoke-WebRequest -Uri http://127.0.0.1:6333/collections/ai_task_orchestrator_chunks -UseBasicParsing
```

期望：

- collection 存在
- vector `size` 为 **128**（mock embedding dimension）
- distance 为 **Cosine**

---

## 8. 可选：JUnit 手工集成测试

测试类：`QdrantManualIntegrationTest`

仅在设置环境变量时运行（**默认 `mvn test` 会跳过**）：

```powershell
cd E:\code\ai-task-orchestrator
$env:QDRANT_MANUAL_VERIFICATION = "true"
docker compose -f docker-compose.qdrant.yml up -d
.\mvnw.cmd test -Dtest=QdrantManualIntegrationTest
Remove-Item Env:QDRANT_MANUAL_VERIFICATION
```

该测试直接对真实 Qdrant 执行 upsert + search，不依赖 MySQL。

---

## 9. 与默认测试的关系

| 命令 | 是否连接 Qdrant |
| --- | --- |
| `.\mvnw.cmd test` | 否（`QdrantVectorStoreTest` 使用 fake client） |
| `QDRANT_MANUAL_VERIFICATION=true` + `QdrantManualIntegrationTest` | 是（手工 opt-in） |

---

## 10. 常见问题

### Qdrant 连接失败

- 确认 `docker compose -f docker-compose.qdrant.yml ps` 中容器运行中
- 确认 `6333` 端口未被占用
- 确认 Spring 启用了 `qdrant` profile

### Collection dimension 不匹配

- mock embedding dimension 为 **128**
- 若曾用其他 provider 写入同一 collection，请 `docker compose -f docker-compose.qdrant.yml down -v` 后重试，或修改 `collection-name`

### Search 无结果

- 先执行 `POST /documents/{documentId}/embeddings`
- `documentId` 过滤需与 embed 文档一致
- query 应与文档内容相关

### 未启用 qdrant profile

仅改 `application.properties` 而不加 profile 时，默认仍为 `app.vector-store.provider=exact`。手工验证必须加 `qdrant` profile。

---

## 11. 本指南不覆盖

- Qdrant 生产部署 / 高可用
- 与主 `docker-compose.yml` 合并（本版本保持独立文件）
- OpenAI / local-worker embedding 与 Qdrant 的组合验证（可自行扩展，但需注意 dimension）
- VectorStore benchmark 自动化（仍为测试 harness）

---

## 相关文件

- [docker-compose.qdrant.yml](../../docker-compose.qdrant.yml)
- [application-qdrant.properties](../../src/main/resources/application-qdrant.properties)
- [docs/demo/qdrant-flow.http](../demo/qdrant-flow.http)
- [docs/local-dev.md](../local-dev.md)
