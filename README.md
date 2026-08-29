# MindWeaver

> 让值得留下的经历、知识与思考，不再被时间带走。

## 产品愿景

我们相信，在 AI 时代，人的长期记忆需要被重新定义。

人的生物记忆会在数十年的人生中逐渐模糊，但一个人主动留下的经历、知识、
思考与选择，不应只是散落的数据。它们应该能够持续积累、彼此关联，并在需要时
被重新唤起。

MindWeaver 的长期目标，是成为一个由用户掌控、能够伴随一生的外置大脑。
它不只是保存文件或回答问题，而是将用户留下的数据连同时间、来源和上下文，
逐渐编织成连续、可追溯、可理解的个人长期记忆。

多年以后，用户找回的不只是一条记录，还包括它为何重要、与什么相关，以及
自己如何一路走到今天。

MindWeaver 希望扩展人的记忆，而不是替代人的判断；让值得保留的记忆不因时间
流逝而无意消失，同时始终由用户决定什么被记住、修正或删除。

## 技术愿景

“外置大脑”不能只是一个更大的知识库，或拥有更多上下文的聊天机器人。
MindWeaver 要把个人记忆建设成一层独立于具体模型与服务、能够长期存在的
数据基础设施。

项目将围绕以下原则持续演进：

- **用户主权与本地优先**：记忆数据优先保存在用户控制的 Vault 中，不被特定
  云服务、账户或供应商锁定。
- **面向长期保存**：通过版本化格式、完整性校验、备份恢复和明确的数据迁移
  机制，让记忆能够跨越设备更换、软件升级与技术迭代。
- **真实且可追溯**：回答、摘要和关系应能够回到原始内容及其时间、来源和变更
  记录；原始事实与模型推断必须被明确区分。
- **理解时间与上下文**：记忆不只是孤立的文本，还应保留事件、人物、项目、
  主题和经历之间的关系，以及这些关系如何随时间变化。
- **模型可替换，记忆可延续**：模型、索引和算法可以升级或重建，但用户的长期
  记忆不应随某项技术的淘汰而消失。
- **记忆由用户决定**：系统不应擅自替用户遗忘，也不能剥夺用户检查、纠正、
  导出和主动删除记忆的权利。

这是 MindWeaver 的长期演进方向，不是对当前版本能力的声明。现阶段，
MindWeaver 正在建设本地 Vault、可靠持久化、可追溯检索和备份恢复等基础能力；
当前已经实现的范围与限制见下文。

## 当前实现

MindWeaver 当前是面向 Windows 10/11 x64 的单用户、本地优先知识工作台。
当前产品实现位于 [`v2/`](v2/README.md)，使用 Go、SQLite、不可变 Blob 与
内嵌 loopback Web UI；它不依赖 Java、MySQL、RabbitMQ 或 Python worker。

本仓库仍处于开发和资格验证阶段，不是已签名或完成安装验证的正式发布。
准确的完成状态以
[`acceptance-ledger.md`](docs/rewrite/acceptance-ledger.md) 为准。
Java 中值得保留的领域逻辑和后端能力已经按 Go-owned 语义收口；范围、审计依据
和未包含的发布资格见
[`java-domain-backend-completion.md`](docs/rewrite/java-domain-backend-completion.md)。

## 当前 CORE 产品边界

- 首次创建、随后重开并独占一个本地 Vault；不读取或迁移旧 Java/MySQL 数据。
- 上传不超过 4 MiB 的 TXT、Markdown 和带真实文本层的 PDF。
- 管理文档、集合成员关系、垃圾箱、恢复和永久清理；内部 revision 仅用于冲突保护。
- 在指定集合范围内使用 SQLite FTS5 检索一段连续原文短语。
- 可选连接固定 loopback Ollama，在检索命中后生成带来源引用的回答。
- 在运行中的 UI 创建不可覆盖的明文备份；验证和恢复仅作为互斥的启动命令。
- 仅构建 `mindweaver.exe` 与 `mindweaver-pdf.exe` 两个 Windows PE。

检索不是自然语言语义搜索。系统把完整输入作为一段连续原文短语，要求至少
3 个 Unicode 码点且不超过 1024 个 UTF-8 字节；不做分词、同义词扩展、
query rewrite、向量搜索、混合召回或 rerank。Ask 使用同一个检索边界，
没有命中时应拒答，而不是把一般自然问题描述成已经支持。

Vault 和备份没有应用层加密。其保密性依赖当前 Windows 账户、文件访问控制
和用户选择的磁盘加密；现有完整性验证不能被描述为加密保证。

## 开发构建与运行

需要冻结的 Go 1.27.0 Windows/amd64 工具链。`go.mod` 保留 Go 1.26
语言/模块指令。首次构建需要联网下载模块；`go.sum` 校验模块内容，下载完成后
可复用 Go 模块缓存。构建时联网不改变应用运行时的本地优先边界。

```powershell
Set-Location .\v2
$go = 'C:\path\to\go1.27.0\bin\go.exe'
New-Item -ItemType Directory -Force .\dist | Out-Null
& $go mod download
& $go mod verify
& $go build -mod=readonly -trimpath -buildvcs=false -o .\dist\mindweaver.exe .\cmd\mindweaver
& $go build -mod=readonly -trimpath -buildvcs=false -o .\dist\mindweaver-pdf.exe .\cmd\mindweaver-pdf
& .\dist\mindweaver.exe serve -config .\mindweaver.v1.json -vault .\vault
```

首次 `serve` 会创建缺失的版本化配置。打开程序输出的一次性
`127.0.0.1` URL；PDF 摄取要求两个 PE 位于同一目录。Ollama 是可选的，
未配置或不可用时文档管理和连续短语检索仍可使用。

备份恢复是 startup-only 模式，永不覆盖或合并已有 Vault：

```powershell
& .\dist\mindweaver.exe recovery verify -backup C:\absolute\backup
& .\dist\mindweaver.exe recovery restore -backup C:\absolute\backup -vault C:\absolute\new-vault
```

备份目标和恢复目标应是用户明确提供、由当前账户控制的本地固定卷绝对路径。

## 构建验证

从 `v2/` 运行：

```powershell
.\scripts\ci.ps1 -Go C:\path\to\go1.27.0\bin\go.exe
.\scripts\verify-standalone.ps1 -Go C:\path\to\go1.27.0\bin\go.exe
.\scripts\test-browser.ps1 -GoExecutable C:\path\to\go1.27.0\bin\go.exe -SelfTest
```

这些脚本先执行 `go mod download` 与 `go mod verify`，再以
`-mod=readonly` 验证 Go 格式、测试、vet、双 PE 构建与 tracked-only 独立
抽取。CI 可缓存由 `go.sum` 绑定的模块；首次构建或缓存未命中需要网络。
通过开发门禁不等于安装、签名、真实浏览器或 clean-VM 发布资格已经完成。

## 明确未实现或未发布

首个 Go CORE 不包含自然问题理解、Embedding/向量检索、Hybrid/RRF、
rerank、reindex、OCR/扫描 PDF、Agent、Memory、Batch、Evaluation、
Qdrant、云模型供应商、插件或内置更新器。这些能力不得从旧实现恢复，只有在
以后从零设计并通过独立验收后才可能进入产品。

签名 MSI、项目许可证、最终 SBOM/NOTICE、Authenticode/ICE、N-1/N-2
升级/回滚以及 clean non-admin VM 安装/卸载矩阵尚未闭合。仓库中的开发构建
不得被称为已安装、已签名或可发布产品。

## 冻结的历史目录

全部旧 Java 时代材料已经集中到
[`legacy/java/`](legacy/java/README.md)：包括 Java/Spring、Maven、Python
worker、旧 Compose 拓扑、旧开发/面试/API 文档和 IDE 元数据。该目录仅用于
删除式审查、需求溯源和历史恢复，不进入 Go 构建、运行、CI 或发布，也不是
受支持的数据迁移入口。目录内说明和配置可能描述已删除能力或不安全旧默认值，
不能作为现行产品说明或执行指令。

不要运行 Maven、旧 Docker Compose 或 Python worker 来启动 MindWeaver。
不要从旧数据库、队列、向量库或历史文件导入用户数据；应在新的 Go Vault 中
重新上传受支持的源文件。

现行架构决策与验收总账位于 [`docs/rewrite/`](docs/rewrite/README.md)，并由其
链接到 `v2/` 中的资格测试、包测试、OpenAPI 合同与脚本；开发入口、限制与
命令以 [`v2/README.md`](v2/README.md) 为准。
