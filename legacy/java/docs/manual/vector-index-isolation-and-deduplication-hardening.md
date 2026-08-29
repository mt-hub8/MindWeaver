# V16.0 向量索引隔离与去重防污染强化

## 目标

V16.0 将 RAG 隔离从「查询时加 `collection_id` filter」前移到**向量写入、更新、删除、重建与审计**全链路，防止：

- 重复导入 / 批量 retry 产生重复向量
- reindex 后旧 generation 向量残留
- PURGED 文档向量残留
- 不同 collection 的向量混入同一存储范围
- embedding 模型 / 维度切换后向量混写

## 核心模块

| 模块 | 职责 |
|------|------|
| `VectorIdentityService` | 由 `collection_id`、`document_id`、`chunk_uid`、`embedding_model`、`embedding_dimension`、`generation` 生成稳定 `vector_id` |
| `VectorPayloadBuilder` | 标准化 vector payload，集中校验必填字段与维度 |
| `IdempotentVectorUpsertService` | 幂等 upsert，同一 `vector_id` 重复写入返回 `UPDATED` |
| `VectorNamespaceGuard` | 写入前校验 collection / document / generation / 生命周期隔离 |
| `VectorGenerationService` | BUILDING / ACTIVE / RETIRED / FAILED 代际管理 |
| `VectorLifecycleSyncService` | trash / restore 同步 embedding payload status |
| `VectorCleanupService` | 按 scope 清理 orphan、retired generation、purged residue、跨集合污染 |
| `VectorConsistencyAuditService` | 只读审计：重复、孤儿、缺失、跨集合、错误代际、维度不一致、永久删除残留 |
| `CollectionPollutionAuditService` | collection 级污染报告与优化建议 |

## Vector Identity

```text
stable_vector_key = sha256(collection_id + document_id + chunk_uid + embedding_model + embedding_dimension)
vector_id         = sha256(stable_vector_key + generation)
```

同一 chunk 同一 generation 重复处理不会新增 vector count；不同 generation 或不同 embedding 模型会生成新的 identity。

## 标准 Payload 字段

必填：

```text
collection_id
document_id
chunk_uid
embedding_model
embedding_dimension
vector_generation
status
```

完整 payload 还包含 `stable_vector_key`、`content_hash`、`metadata_hash`、`section_path`、`chunk_type` 等，便于过滤与审计。

## Generation 代际隔离

reindex 流程：

```text
1. 创建 generation=N+1，status=BUILDING
2. 新 chunk / vector 写入新 generation
3. 校验通过后切换 ACTIVE
4. 旧 generation 标记 RETIRED
5. cleanup retired generation
```

BUILDING 不参与默认检索；FAILED 不破坏旧 ACTIVE generation。

## 文档生命周期联动

| 状态 | Vector 行为 |
|------|-------------|
| ACTIVE | payload status=ACTIVE，可检索 |
| TRASHED | 更新 payload status=TRASHED，默认过滤，不立即删除向量 |
| RESTORE | 恢复 payload status=ACTIVE，无需重新 embedding |
| PURGED | 物理删除 vector，cleanup 可发现残留 |

## Batch Retry 防重复

V16 规则：

- batch item retry 复用同一 `documentId` / `chunkUid` / `generation`
- `retryCount` 不参与 vector identity
- 同一 chunk 已写入时再次 upsert 为 `UPDATED`
- duplicate file skip 不写 vector
- `IMPORT_ANYWAY` 使用新 `documentId`，允许新 vector

## 审计指标

| 指标 | 含义 |
|------|------|
| **VectorDuplicateRate** | 同一 `stableVectorKey` 对应多个向量的比例 |
| **VectorOrphanRate** | 向量存在但 chunk 已不存在 |
| **VectorMissingRate** | ACTIVE chunk 缺少对应向量 |
| **CrossCollectionVectorLeakRate** | 目标 collection 范围内混入其他 collection 的向量 |
| **WrongGenerationVectorRate** | 向量 generation 与 active generation 不一致 |
| **ModelDimensionMismatchRate** | payload 维度与实际向量长度不一致 |
| **TrashedVectorVisibleRate** | TRASHED 文档向量仍可能被默认检索召回 |
| **PurgedVectorResidueRate** | PURGED 文档仍残留向量 |

### 与 V14 CrossCollectionLeakRate 的区别

- **V14 CrossCollectionLeakRate**：检索结果层污染率（TopK / final context 是否跨 collection）
- **V16 CrossCollectionVectorLeakRate**：向量存储层污染率（底层 vector payload 是否串库）

## API

### 查询与审计

```http
GET  /vector-index/summary
POST /vector-index/audit
GET  /vector-index/audits
GET  /vector-index/audits/{auditRunId}
GET  /vector-index/audits/{auditRunId}/issues
POST /vector-index/collections/{collectionId}/audit
POST /vector-index/documents/{documentId}/audit
```

### 清理（需明确 scope）

```http
POST /vector-index/cleanup/orphans
POST /vector-index/cleanup/retired-generations
POST /vector-index/cleanup/purged-residue
POST /vector-index/collections/{collectionId}/cleanup-pollution
```

audit 为只读诊断；cleanup 为破坏性操作，需用户确认后执行。

## 页面

路径：`/vector-index-health.html`

页面名称：**向量索引健康**

关键能力：

- 概览 vector / chunk / collection / active generation 数量
- 按 collection 运行污染审计
- 展示重复向量率、跨集合向量污染率、孤儿 / 缺失 / 错误代际 / 永久删除残留
- 提供孤儿向量与 retired generation 清理入口
- 技术详情折叠查看原始 JSON

知识库体检页（`/knowledge-health.html`）提供跳转链接，便于从检索层诊断进入向量存储层排查。

## 与 V12 / V13 / V14 / V15 联动

- **V12**：PURGED 触发 vector cleanup；audit 发现 purged residue
- **V13**：批量 retry 不产生 duplicate vector
- **V14**：CrossCollectionLeakRate 高时，建议运行 Vector Index Health 判断是检索 filter 还是存储层污染
- **V15**：RetrievalDiagnostics 提示 wrong collection / wrong generation 时，可运行 vector audit 进一步定位

## 测试说明

V16 单元测试不依赖 Qdrant / Ollama / Docker，使用 Mockito 与内存 fixture 覆盖 identity、payload、upsert、namespace guard、generation、cleanup、audit 与 controller。
