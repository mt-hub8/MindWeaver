# MindWeaver

MindWeaver 是面向 Windows 10/11 x64 的单用户、本地优先知识工作台。
当前产品实现位于 [`v2/`](v2/README.md)，使用 Go、SQLite、不可变 Blob 与
内嵌 loopback Web UI；它不依赖 Java、MySQL、RabbitMQ 或 Python worker。

本仓库仍处于开发和资格验证阶段，不是已签名或完成安装验证的正式发布。
准确的完成状态以
[`acceptance-ledger.md`](docs/rewrite/acceptance-ledger.md) 为准。

## 当前 CORE 产品边界

- 创建并独占一个新的本地 Vault；不读取或迁移旧 Java/MySQL 数据。
- 上传不超过 4 MiB 的 TXT、Markdown 和带真实文本层的 PDF。
- 管理文档、修订、集合成员关系、垃圾箱、恢复和永久清理。
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
语言/模块指令，依赖从已审查的 `vendor/` 离线解析。

```powershell
Set-Location .\v2
$go = 'C:\path\to\go1.27.0\bin\go.exe'
New-Item -ItemType Directory -Force .\dist | Out-Null
& $go build -mod=vendor -trimpath -buildvcs=false -o .\dist\mindweaver.exe .\cmd\mindweaver
& $go build -mod=vendor -trimpath -buildvcs=false -o .\dist\mindweaver-pdf.exe .\cmd\mindweaver-pdf
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

## 离线验证

从 `v2/` 运行：

```powershell
.\scripts\ci.ps1 -Go C:\path\to\go1.27.0\bin\go.exe
.\scripts\verify-standalone.ps1 -Go C:\path\to\go1.27.0\bin\go.exe
.\scripts\test-browser.ps1 -GoExecutable C:\path\to\go1.27.0\bin\go.exe -SelfTest
```

这些脚本使用空 `GOMODCACHE`/`GOCACHE`、`-mod=vendor` 和禁网模块策略，
并验证 Go 格式、测试、vet、双 PE 构建与 tracked-only 独立抽取。通过开发
门禁不等于安装、签名、真实浏览器或 clean-VM 发布资格已经完成。

## 明确未实现或未发布

首个 Go CORE 不包含自然问题理解、Embedding/向量检索、Hybrid/RRF、
rerank、reindex、OCR/扫描 PDF、Agent、Memory、Batch、Evaluation、
Qdrant、云模型供应商、插件或内置更新器。这些能力不得从旧实现恢复，只有在
以后从零设计并通过独立验收后才可能进入产品。

签名 MSI、项目许可证、最终 SBOM/NOTICE、Authenticode/ICE、N-1/N-2
升级/回滚以及 clean non-admin VM 安装/卸载矩阵尚未闭合。仓库中的开发构建
不得被称为已安装、已签名或可发布产品。

## 冻结的历史目录

以下内容仅作为删除式审查和需求溯源的历史材料，不进入 Go 构建、运行、CI
或发布，也不是受支持的数据迁移入口：

- `src/`、`.mvn/`、`mvnw*`、`pom.xml`：旧 Java/Spring 实现；
- `workers/`：旧 Python worker；
- `docker-compose*.yml`：旧 MySQL、RabbitMQ、Qdrant 开发拓扑；
- `docs/manual/`、旧开发/面试/API 文档：可能描述已删除能力，不能作为现行产品说明。

不要运行 Maven、旧 Docker Compose 或 Python worker 来启动 MindWeaver。
不要从旧数据库、队列、向量库或历史文件导入用户数据；应在新的 Go Vault 中
重新上传受支持的源文件。

现行架构决策和证据只在 [`docs/rewrite/`](docs/rewrite/README.md)；开发入口、
限制与命令以 [`v2/README.md`](v2/README.md) 为准。
