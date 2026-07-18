# V19.0 Memory Foundation & Agent Profiles

## 阶段目标

V19.0 的中文阶段名是“记忆机制底座与智能体角色档案”。目标是在 MindWeaver / Personal AI Knowledge Workspace 中建立一层可控、可追溯、可分作用域、可关闭的 Memory，并为后续 V20 多智能体编排准备 Agent Profile、Agent Memory 和 Shared Memory。

本阶段不做多智能体编排、Agent 辩论、Skill 系统、人格进化、桌面打包或联网搜索，也不会默认自动长期记忆所有对话。

## 为什么先做记忆，再做多 Agent

如果没有稳定的记忆边界，多 Agent 会放大几个问题：

- 不同角色可能互相读取不该读取的私有结论；
- 旧项目状态、低置信度摘要和冲突偏好会污染后续任务；
- 用户无法知道某个结论来自哪里，也无法修正或删除；
- 长文档、RAG chunk 与短期决策混在一起，导致上下文和引用失真。

V19 先固定数据模型、来源、可信度、过期时间、作用域和用户控制面。V20 只能在这些边界之上做编排，不应绕过它们。

## Memory 与知识库的边界

`memory_item` 保存短而结构化的信息，例如偏好、约束、决策、项目状态和任务总结。API 将内容限制为 8000 个字符。

长文档、完整会议记录、论文、规范和大段原文必须进入知识库，由文档摄入、structured chunk、hybrid retrieval 和 Grounded Answer 链路处理。Memory 不默认向量化，也不进入知识库 chunk 表。

这两个上下文通道在 Prompt 和响应中分开展示：

```text
【长期记忆】
偏好、约束、项目状态、角色私有记忆

【知识库资料】
RAG 检索到的、可生成 citation 的文档 chunk
```

Memory 不能伪造 citation。V18 的 citation verification 仍然只验证 `GroundedContextBundle` 中的知识库 chunk。

## Memory 类型

| MemoryType | 中文含义 | 典型内容 |
|---|---|---|
| `PREFERENCE` | 用户偏好 | 回答语言、格式偏好 |
| `FACT` | 事实 | 经用户确认的短事实 |
| `DECISION` | 决策 | 已确认的技术或产品决策 |
| `SUMMARY` | 摘要 | 经确认的短摘要 |
| `CONSTRAINT` | 约束 | 不得联网、兼容性边界 |
| `FEEDBACK` | 反馈 | 用户对结果的明确反馈 |
| `TASK_RESULT` | 任务结果 | 用户确认保存的任务总结 |
| `PROJECT_STATE` | 项目状态 | 当前阶段、里程碑、阻塞 |
| `AGENT_INSTRUCTION` | Agent 指令 | 某角色的长期工作约束 |
| `RISK_NOTE` | 风险记录 | 已识别的风险与观察 |

`PREFERENCE` 必须提供 `memory_key`。相同作用域和绑定条件下，ACTIVE 的同 key 偏好会更新原记录，而不是重复新增。

## Memory Scope

| MemoryScope | 用途 | 默认可见范围 |
|---|---|---|
| `USER` | 用户级记忆 | Ask 和各 Agent 可作为公共上下文 |
| `PROJECT` | 项目级记忆 | 仅匹配 `project_id` 时使用 |
| `AGENT` | Agent 私有记忆 | 仅匹配 `agent_profile_id` 时使用 |
| `TASK` | 任务级记忆 | 仅匹配 `task_id` 时使用 |
| `SHARED` | 共享记忆 | 可被多个 Agent 使用 |

`AGENT` 必须绑定 `agent_profile_id`，`PROJECT` 必须绑定 `project_id`，`TASK` 必须绑定 `task_id`。这些约束在 Service 层校验。

Visibility 进一步标记 `PRIVATE`、`SHARED` 或 `SYSTEM`。Shared Memory 必须显式使用 `memory_scope=SHARED` 与 `visibility=SHARED`，不能仅凭标题或 metadata 推断。

## 数据模型

Flyway `V33__memory_foundation_and_agent_profiles.sql` 新增：

- `agent_profile`
- `memory_item`
- `agent_task.agent_profile_id`

`memory_item` 的核心字段：

- 标识与正文：`id`、`memory_key`、`title`、`content`
- 分类边界：`memory_type`、`memory_scope`、`visibility`、`status`
- 来源追踪：`source_type`、`source_id`
- 作用域绑定：`project_id`、`agent_profile_id`、`task_id`
- 质量与生命周期：`confidence`、`importance`、`expires_at`、`last_used_at`、`use_count`
- 审计：`created_at`、`updated_at`、`deleted_at`、`metadata_json`

`confidence` 范围是 0–1，数据库使用 `DECIMAL(5,4)`；`importance` 范围是 0–100。删除采用 `status=DELETED` 加 `deleted_at` 的软删除。

Memory 不对 Agent Profile 建外键，这是有意设计：删除 Profile 后相关 Memory 不会被级联删除，诊断可以将其标为 Orphan Agent Memory。`agent_task.agent_profile_id` 使用 `ON DELETE SET NULL`，保留历史任务。

## Memory 写入规则

默认规则：

1. 用户在记忆中心手动保存时直接写入；
2. Ask 回答默认不自动保存；
3. Agent Task 完成后只提供保存 API，必须提交 `confirmed=true`；
4. 系统未来若建议保存，也必须由用户确认；
5. 相同 `memory_key` 的偏好更新而不是重复新增；
6. 删除、过期和冲突记忆默认不进入上下文；
7. 低置信度记忆可以进入，但必须显示 warning；
8. 超长内容拒绝进入 Memory，提示改用知识库。

为什么不自动记忆所有内容：对话中可能包含临时想法、敏感信息、模型幻觉、未确认事实和已经失效的状态。无差别自动写入会把这些内容变成长效上下文，既不可控也难以追溯。

## Memory Service

`MemoryService` 支持：

- `createMemory`
- `updateMemory`
- `archiveMemory`
- `deleteMemory`
- `restoreMemory`
- `listMemories`
- `searchMemories`
- `getRelevantMemories`
- `markMemoryUsed`
- `detectConflicts`
- `detectExpiredMemories`
- `detectLowConfidenceMemories`
- Agent Memory 的 `share` / `unshare`

主要 API：

```text
POST   /memories
GET    /memories
GET    /memories/{id}
PUT    /memories/{id}
DELETE /memories/{id}
POST   /memories/{id}/archive
POST   /memories/{id}/restore
POST   /memories/{id}/resolve-conflict
GET    /memories/relevant
GET    /memories/diagnostics
GET    /memories/settings
POST   /memories/settings/enabled?enabled=false
```

关闭记忆只停止自动上下文读取，不删除已有数据。配置文件中的默认值：

```properties
app.memory.enabled=true
app.memory.max-context-items=8
app.memory.min-confidence=0.5
app.memory.include-expired=false
app.memory.include-conflicted=false
```

运行时开关重启后会回到配置值；如需持久关闭，应修改配置。

## Agent Profile 设计

`agent_profile` 保存：

- `agent_key`
- `display_name`
- `role_name`
- `description`
- `system_instruction`
- `default_memory_scope`
- `enabled`
- 创建/更新时间和 `metadata_json`

V33 写入四个默认角色：

1. `ProductAgent` / 产品经理 Agent：用户价值、功能边界、页面体验、需求优先级；
2. `ArchitectAgent` / 架构师 Agent：模块边界、数据模型、接口设计、代码债风险；
3. `RagEngineerAgent` / RAG 工程 Agent：检索链路、chunk、metadata、评测指标、向量库；
4. `RiskReviewerAgent` / 风险审查 Agent：过度设计、性能风险、测试缺口、安全边界。

Profile API：

```text
POST   /agent-profiles
GET    /agent-profiles
GET    /agent-profiles/{id}
PUT    /agent-profiles/{id}
DELETE /agent-profiles/{id}
POST   /agent-profiles/{id}/enable
POST   /agent-profiles/{id}/disable
```

## Agent 私有记忆与 Shared Memory

Agent Memory API：

```text
GET  /agent-profiles/{id}/memories
POST /agent-profiles/{id}/memories
POST /agent-profiles/{id}/memories/{memoryId}/share
POST /agent-profiles/{id}/memories/{memoryId}/unshare
```

隔离规则：

- Agent A 查询时只能自动读取 `agent_profile_id=A` 的 AGENT Memory；
- Agent B 的私有 Memory 会被过滤；
- USER 与匹配的 PROJECT Memory 可作为公共背景；
- SHARED Memory 可供多个 Agent 使用；
- share / unshare 必须由显式 API 调用；
- share 后仍保留原 `agent_profile_id`，用于确认所有者和取消共享。

## Memory Context Assembly

`MemoryContextAssembler` 输入 query、projectId、agentProfileId、taskId、scopes 和 limit，输出：

- `memories`
- `usedMemoryCount`
- `skippedMemoryCount`
- `warnings`

第一版不使用向量。筛选与排序顺序是：

1. 检查全局开关；
2. 默认只允许 ACTIVE；
3. 排除实际已过期、DELETED、ARCHIVED 和 CONFLICTED；
4. 校验 PROJECT / AGENT / TASK 绑定；
5. 应用显式 scopes；
6. 按关键词、importance、confidence 和更新时间综合排序；
7. 最多使用配置的 8 条；
8. 标记使用时间与次数；
9. 低于 `min-confidence` 时写入 warning。

Memory 数量与知识库 `max-context-chars` 分开控制，不会通过提高 Memory limit 挤占或绕过 RAG 的 final context。

## Ask 如何使用 Memory

Ask 请求可以传入：

- `projectId`
- `agentProfileId`
- `taskId`
- `memoryScopes`
- `memoryLimit`

主链路是：

```text
Query Understanding
  -> MemoryContextAssembler
  -> Retrieval Routing
  -> RAG Retrieval
  -> Grounded Answer
  -> Citation Verification
```

响应新增独立 `memoryContext`。Prompt 中先放“长期记忆”，再放“知识库资料”。Memory 可以改变回答语言、格式、约束和项目背景；事实性回答仍必须由 RAG 文档支撑。没有文档支持的事实只能标成“记忆信息”，不能生成 citation。

## Agent Task 如何使用 Memory

创建任务时 `CreateAgentTaskRequest` 可传 `agentProfileId`。创建阶段会验证 Profile 存在且启用。最终报告 Prompt 包含：

- Agent Profile 的 `system_instruction`
- 该 Agent 可见的 USER / AGENT / SHARED / TASK Memory
- 原有知识库检索结果和 citation

任务完成后不会自动保存。用户确认后调用：

```text
POST /agent/tasks/{taskId}/memory-summary
{
  "confirmed": true,
  "title": "可选标题",
  "confidence": 0.8,
  "importance": 60
}
```

系统把已完成任务结果保存为 `TASK_RESULT`、`TASK` scope、`AGENT_TASK` source，并记录 `userConfirmed=true`。

## Memory Diagnostics

`MemoryDiagnosticsService` 检查：

1. Expired Memory：状态过期或 `expires_at` 已到；
2. Low Confidence Memory：低于配置阈值；
3. Conflict Memory：同 key 或同标题/类型/作用域下内容不一致；
4. Duplicate Memory：类型、作用域、绑定和内容相同；
5. Orphan Agent Memory：绑定的 Profile 不存在；
6. Overused Memory：使用次数达到阈值；
7. Stale Project Memory：项目状态超过 90 天未更新。

报告包含计数、issues 和中文 suggestions，例如归档旧项目状态、合并重复偏好、确认冲突、删除低置信度自动摘要。

## 页面

### 记忆中心

路径：`/memory-center.html`

能力：

- User / Project / Agent / Shared / 冲突 / 低置信度 / 过期统计；
- 类型、作用域、Agent、状态、来源和关键词筛选；
- 新增、编辑、软删除、归档、恢复；
- share / unshare；
- 标记冲突已解决；
- 查看来源、可信度、重要性和技术详情；
- 查看诊断与优化建议；
- 关闭或启用 Memory Context 读取。

### 智能体角色

路径：`/agent-profiles.html`

能力：

- 展示四个默认角色与自定义角色；
- 查看名称、职责、system instruction 摘要、启用状态；
- 查看私有/共享记忆数量；
- 新建、编辑、启用、禁用和删除角色；
- 跳转查看或添加角色记忆。

页面明确提示：“多智能体协作将在后续阶段使用这些角色档案。”

## 当前边界

V19 不做：

- 多 Agent 编排或辩论；
- 自动保存所有消息；
- 敏感信息自动识别与长期保存；
- Memory embedding 或复杂 memory graph；
- Skill 生成或人格进化；
- 联网搜索；
- React / Vue / npm 前端；
- 桌面打包。

## FAQ

### Memory 是另一套知识库吗？

不是。Memory 保存短偏好、约束、决策和状态；知识库保存长文档并负责 RAG citation。

### 为什么事实 Memory 不能直接当 citation？

Memory 的来源可能是手动输入、任务摘要或历史回答，不等同于可核验文档证据。citation 只指向 Grounded Context 中的知识库 chunk。

### 关闭记忆会删除数据吗？

不会。关闭只停止 Ask / Agent Task 的自动读取。记忆仍可在记忆中心查看、编辑和删除。

### 任务完成后会自动留下长期记忆吗？

不会。必须由用户明确调用保存接口并提交 `confirmed=true`。

### Agent A 能读取 Agent B 的私有记忆吗？

不能。只有显式转为 SHARED 后，其他 Agent 才能读取。

### 删除 Agent Profile 会发生什么？

历史任务保留，`agent_profile_id` 会置空；Memory 不级联删除，并在诊断中显示为 Orphan Agent Memory，等待用户重新绑定、共享或删除。

### 当前相关记忆检索是否需要 Qdrant？

不需要。V19 使用数据库过滤、关键词、重要性、可信度和新近度排序，默认测试不依赖 Qdrant、Ollama、Python Worker、Docker 或外部 API。
