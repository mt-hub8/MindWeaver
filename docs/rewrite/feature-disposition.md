# Legacy feature disposition

Statuses:

- `KEEP_SEMANTICS`: preserve the user-observable contract.
- `REDESIGN`: preserve the user need but replace the implementation and failure semantics.
- `REBUILD`: recreate the capability from new Go-owned canonical input and
  regenerate derived data.
- `DEFER`: retain the requirement but do not block the core replacement release.
- `DROP`: remove intentionally with a documented data disposition.

| ID | Legacy capability | Disposition | Salvage decision | First release scope | Go target | Required acceptance evidence |
| --- | --- | --- | --- | --- | --- | --- |
| MW-DOC-001 | TXT/Markdown/text-PDF upload | REDESIGN | CORE_REBUILD_FROM_ZERO | CORE | upload, blob, ingestion | duplicate, cancel, crash, retry, source hash and locator E2E |
| MW-DOC-002 | Document lifecycle | REDESIGN | CORE_REBUILD_FROM_ZERO | CORE | document/generation | ACTIVE/TRASHED/PURGE_PENDING/PURGED state model tests |
| MW-DOC-003 | Reindex | REDESIGN | LATER_FROM_ZERO | LATER | generation coordinator | old generation remains queryable; per-document atomic activation |
| MW-DOC-004 | Deterministic chunks and FTS indexing | REBUILD | CORE_REBUILD_FROM_ZERO | CORE | chunking/SQLite FTS pipeline | deterministic golden chunks, transactional FTS refresh and lifecycle filtering |
| MW-COL-001 | Collections | KEEP_SEMANTICS | CORE_REQUIREMENT_ONLY | CORE | collection membership | true many-to-many scope; empty scope never becomes global |
| MW-RET-001 | Literal continuous-phrase lexical retrieval | REDESIGN | CORE_REBUILD_FROM_ZERO | CORE | controlled SQLite FTS retrieval | three-plus-code-point Chinese and English source phrases; deterministic order; active lifecycle and exact collection isolation; stable source chunk IDs |
| MW-RET-002 | Natural-question query understanding, hybrid/RRF/rerank/expansion | REDESIGN | LATER_FROM_ZERO | LATER | retrieval stages | representative recall/ranking/FDR, accepted 100k capacity/latency/disk budgets, incremental maintenance and restart conformance before one executable path is selected |
| MW-EMB-001 | Embeddings and vector retrieval | REBUILD | LATER_FROM_ZERO | LATER | future embedding/vector slice | no first-release schema/provider/route; later corpus, capacity and backend conformance |
| MW-RAG-001 | Keyword/continuous-phrase-driven Ask | KEEP_SEMANTICS | CORE_REBUILD_FROM_ZERO | CORE | answer run | the complete input uses the same scoped literal-phrase search; final source chunk IDs and provider failure are recorded honestly; natural-question retrieval is not claimed |
| MW-RAG-002 | Citations | REDESIGN | CORE_REBUILD_FROM_ZERO | CORE | answer citations | every citation resolves to a source chunk supplied for that answer |
| MW-RAG-003 | Honest refusal and labeling | REDESIGN | CORE_REBUILD_FROM_ZERO | CORE | answer policy | no-hit and structurally invalid citation cases refuse or carry an explicit limitation label |
| MW-CON-001 | Conversation history | KEEP_SEMANTICS | CORE_REBUILD_FROM_ZERO | CORE | conversation/message/answer | immutable answer provenance and purge behavior |
| MW-PRO-001 | Optional local Ollama | REDESIGN | CORE_REBUILD_FROM_ZERO | CORE | internal/ollama with versioned non-secret config | real loopback probe/invocation, chosen-config source binding and bounded no-proxy egress |
| MW-JOB-001 | Ordinary Task/outbox | DROP | DROP | DROP | unified durable job | no separate task executor or RabbitMQ path |
| MW-AGT-001 | Agent tasks | REDESIGN | LATER_FROM_ZERO | LATER | agent run on unified job | atomic steps, read-only tools first, budgets, cancellation, crash recovery |
| MW-AGT-002 | Agent profiles | REDESIGN | LATER_FROM_ZERO | LATER | agent/profile | versioned profile updates, deletion lineage and memory-scope enforcement |
| MW-MEM-001 | Memory | REDESIGN | LATER_FROM_ZERO | LATER | explicit memory | opt-in auto-write, source/scope/retention, shared context budget |
| MW-BAT-001 | Batch import | REDESIGN | LATER_FROM_ZERO | LATER | group of ingestion jobs | bounded concurrency, partial failure, cancellation and staging retention |
| MW-NOT-001 | User notifications | REDESIGN | LATER_FROM_ZERO | LATER | durable notification projection | job outcome/recovery notifications are deduplicated, attributable and operable in the UI |
| MW-EVL-001 | Evaluation | REDESIGN | LATER_FROM_ZERO | LATER | versioned evaluation | fixed corpus/dataset fingerprint and same retrieval snapshot as generation |
| MW-VEC-001 | Qdrant | DEFER | LATER_FROM_ZERO | LATER | optional backend | full generation/scope/delete/audit conformance before release |
| MW-HLT-001 | Vector/index health | REDESIGN | LATER_FROM_ZERO | LATER | diagnostics/repair | real backend capabilities, dry-run, backup and repair receipt |
| MW-TRS-001 | Trash/restore | KEEP_SEMANTICS | CORE_REQUIREMENT_ONLY | CORE | retention coordinator | immediate retrieval exclusion and reversible restore |
| MW-TRS-002 | Permanent delete | REDESIGN | CORE_REBUILD_FROM_ZERO | CORE | document deletion | delete known in-scope DB/FTS rows and reference-aware blobs; failures never report success |
| MW-BKP-001 | Backup/restore | REDESIGN | CORE_REBUILD_FROM_ZERO | CORE | backup coordinator | DB snapshot + blob manifest; restore on a clean machine |
| MW-UI-001 | Static management UI | REDESIGN | CORE_REBUILD_FROM_ZERO | CORE | embedded web UI | API contract, browser security, accessibility and recovery workflows |
| MW-CFG-001 | Versioned local configuration | REDESIGN | CORE_REBUILD_FROM_ZERO | CORE | platform/config | explicit allowlist, unknown/duplicate rejection; legacy and cloud secrets are never exported or imported |
| MW-RUN-001 | Local launch, shutdown and environment scripts | REDESIGN | CORE_REBUILD_FROM_ZERO | CORE | single-process runtime and signed Windows package | one executable owns the Vault; install/start/stop/uninstall pass clean-machine tests |
| MW-STO-001 | Storage summary and cache controls | REDESIGN | LATER_FROM_ZERO | LATER | storage diagnostics and repair plans | authoritative/derived bytes are distinguished and destructive actions emit verified receipts |
| MW-CCH-001 | Embedding and retrieval caches | REBUILD | LATER_FROM_ZERO | LATER | derived cache/index stores | cache rows are regenerated only from new Go-owned inputs and fingerprints, never trusted as canonical |
| MW-TSK-001 | Ordinary task history, prompts and results | DROP | DROP | DROP | none | no task-history reader, exporter, importer, or executable continuation exists in the Go product |
| MW-DEV-001 | Development-only task mutation/dispatch endpoints | DROP | DROP | DROP | none | production build exposes no dev mutation or dispatch surface |
| MW-RPT-001 | Legacy evaluation and benchmark report files | DROP | DROP | DROP | none | legacy reports are not packaged or accepted as Go evidence; new gates generate versioned Go reports |
| MW-MIG-001 | Java/MySQL user-data migration | DROP | DROP | DROP | none | no exporter, neutral package, importer, migration CLI, or legacy SQLite schema exists |
| MW-INF-001 | MySQL | DROP | DROP | DROP | SQLite | production/package dependency absence and normal Go SQLite integrity/upgrade checks |
| MW-INF-002 | RabbitMQ | DROP | DROP | DROP | SQLite job queue | atomic claim, one lease token, bounded retry, cancellation and restart recovery |
| MW-INF-003 | Python workers | DROP | DROP | DROP | Go orchestration + Ollama | no Python runtime dependency; parser isolation remains possible |
| MW-INF-004 | Java, Spring and Maven runtime/build stack | DROP | DROP | DROP | standalone Go module and release toolchain | packaged runtime contains no JVM, Maven or Spring dependency |
| MW-INF-005 | Optional local Ollama runtime | KEEP_SEMANTICS | CORE_REBUILD_FROM_ZERO | CORE | versioned Ollama provider adapter | offline capability probe, explicit model selection and no mandatory cloud dependency |
| MW-INF-006 | Legacy Docker Compose development stack | DROP | DROP | DROP | optional test fixtures only | production install does not require Docker; fixture use is isolated from user Vaults |

Every row must eventually link to an ADR, implementation package, data-disposition rule,
tests, user documentation, and release gate. A row cannot be closed solely because
an endpoint or page exists.

`Salvage decision` governs product scope, not source-code reuse. In particular,
`CORE_REQUIREMENT_ONLY` means only the two narrow user requirements survive;
no Java code, schema, identifier, or stored record is reused.
`CORE_REBUILD_FROM_ZERO` means the need belongs in the first release while the
legacy implementation is rejected as a design input. `LATER_FROM_ZERO` is absent
from the first release and may be introduced only by a later, independently
accepted vertical slice. See [`legacy-salvage-review.md`](./legacy-salvage-review.md).
