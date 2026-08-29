# V5.0 知识库范围检索与文档分组

## 目标

V5.0 将系统从「全库问答」升级为「可选择范围的知识库问答」：用户可以创建**知识库分组**（Collection），把文档加入分组，然后在问答时选择**全部文档**或**某个知识库分组**。

本阶段与 V4.0 **知识库生命周期**过滤共同生效，不会互相替代。

## 什么是知识库分组（Collection）

**知识库分组**用于把多份文档组织在一起，例如「项目 A 文档」「运维手册」「产品需求」。

- 一个文档可以加入多个分组；
- 一个分组可以包含多份文档；
- 分组本身不替代文档的启用 / 删除状态；
- 已删除文档可以保留分组归属，但**不会参与问答**。

## 如何创建分组

1. 打开浏览器访问 [`/collections.html`](/collections.html)（页面标题：**知识库分组**）。
2. 填写**分组名称**（必填）与**分组说明**（可选）。
3. 点击**新建分组**。
4. 创建成功后可在分组列表中查看**文档数量**与**可用于问答的文档**数量。

对应 API：`POST /collections`

```json
{
  "name": "项目 A 文档",
  "description": "项目 A 的需求、设计和接口说明"
}
```

## 如何把文档加入分组

### 方式一：文档管理页面

1. 打开 [`/documents.html`](/documents.html)。
2. 在文档列表中找到目标文档。
3. 点击**加入分组**，选择目标**知识库分组**并确认。
4. 成功后文档的**所属分组**列会显示分组名称。

### 方式二：API

`POST /collections/{collectionId}/documents/{documentId}`

- 若文档已在此分组中：幂等成功，返回「该文档已加入此分组」。
- 若文档已删除：允许加入，但会提示「已删除文档不会参与问答」。

## 如何把文档移出分组

### 文档管理页面

点击**移出分组**，选择要移出的分组并确认。

### 分组详情页面

在 [`/collections.html`](/collections.html) 进入**分组详情**，对分组中的文档点击**移出分组**。

### API

`DELETE /collections/{collectionId}/documents/{documentId}`

- 若关系不存在：幂等成功，返回「该文档未加入此分组」。

## Ask 页面如何选择问答范围

打开 [`/ask.html`](/ask.html)：

| 选项 | 行为 |
|------|------|
| **全部文档** | 不传 `collectionId`，在全库已启用文档中检索 |
| **某个知识库分组** | 传 `collectionId`，仅在该分组范围内检索 |

页面会展示：

- **问答范围**
- **当前回答只基于所选分组中的文档**（选择分组时）
- **已删除文档不会进入回答引用**
- **旧版本片段不会进入回答引用**

回答区域会显示**当前问答范围**（全部文档 / 分组名称）。

## 全部文档 vs 指定分组

| 场景 | 检索范围 |
|------|----------|
| 不传 `collectionId` | 全部**已启用**且索引就绪的文档 |
| 传 `collectionId` | 仅该分组内**已启用**、有**当前有效片段**的文档 |

两种模式都会过滤：

- `DELETED` 文档；
- 非当前索引版本（`current_generation`）的旧片段；
- 已失效（`SUPERSEDED`）的 chunk。

## 范围检索（Scoped Retrieval）过滤规则

当传入 `collectionId` 时，系统必须同时满足：

1. 文档属于该分组；
2. 文档生命周期为 **ACTIVE**（已启用）；
3. chunk 为当前有效片段；
4. 文档未软删除；
5. chunk 未失效。

### 为什么还需要应用层过滤（Application-level Filter）

向量库检索（dense）或关键词检索（lexical）返回的候选，**不一定**能可靠地按分组过滤。因此系统在以下环节之前都会做**应用层过滤**：

- 最终 TopK；
- Citations（引用）；
- Prompt 上下文；
- 检索 metadata 中的最终结果。

覆盖路径包括：**dense**、**lexical**、**hybrid 融合**、**rerank**、**citations**、**prompt 构造**、**no-context 判断**。

即使底层 VectorStore 返回了分组外候选，也不会进入最终回答。

## 与 V4.0 生命周期过滤的关系

V5.0 **不替代** V4.0，而是**叠加**：

```
最终可用片段 = 分组范围过滤 ∩ 生命周期过滤（ACTIVE + 当前 generation）
```

- **已删除文档**：即使仍在分组关系中，也不会进入分组问答；
- **旧版本片段**：重新索引后，旧 generation 的片段不会进入回答引用。

## 空分组 / 无可用文档时的行为

| 情况 | 行为 |
|------|------|
| 分组存在但无文档 | 返回 no-context，**不调用 LLM**；提示：当前分组下没有可用于问答的文档 |
| 分组内文档均已删除 | no-context；提示：当前分组中的文档均已删除 |
| 分组有文档但无可用片段 | no-context；提示：请重新建立索引或上传文档 |
| 检索后无匹配片段 | no-context；提示：当前所选分组下未检索到可用于回答的文档片段 |

## API 参考

### RAG 问答（兼容扩展）

`POST /rag/answers`

```json
{
  "query": "Embedding Cache 的 key 是什么？",
  "topK": 5,
  "collectionId": 1
}
```

- `collectionId` **可选**；不传时行为与旧版本一致；
- 响应 `retrieval` 中可增加：`scopeType`、`collectionId`、`collectionName`、`finalContextCount` 等字段，不破坏旧字段。

### 文档列表扩展字段

`GET /documents` 每条文档可增加：

- `collections` / `collectionIds` / `collectionNames`
- `canAssignToCollection` / `canRemoveFromCollection`

## 当前明确不做

- 用户登录 / 权限 / workspace / 多租户
- 文档 ACL / 分组级计费
- 复杂标签系统 / 多级目录 / 分享链接
- 批量导入 / 新上传格式 / 新检索算法

## 常见问题

**Q：删除文档后，分组关系还在吗？**  
A：可以保留。已删除文档不会参与问答，但分组归属可用于管理视图。

**Q：不传 collectionId 会影响旧客户端吗？**  
A：不会。不传时仍为全库检索，仅过滤已删除文档与旧片段。

**Q：Hybrid / Rerank 开启时分组过滤还有效吗？**  
A：有效。所有检索路径在最终上下文组装前都会经过应用层过滤。

**Q：分组名称可以重复吗？**  
A：不可以。系统会返回 `COLLECTION_NAME_DUPLICATED` 错误。

## 相关页面

- 知识库分组：[`/collections.html`](/collections.html)
- 文档管理：[`/documents.html`](/documents.html)
- 知识库问答：[`/ask.html`](/ask.html)
- V4.0 生命周期说明：[knowledge-base-lifecycle-management.md](./knowledge-base-lifecycle-management.md)
