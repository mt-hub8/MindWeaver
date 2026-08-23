# Legacy feature disposition

Statuses:

- `KEEP_SEMANTICS`: preserve the user-observable contract.
- `REDESIGN`: preserve the user need but replace the implementation and failure semantics.
- `REBUILD`: migrate canonical input and regenerate derived data.
- `ARCHIVE_ONLY`: retain readable history without entering new execution state machines.
- `DEFER`: retain the requirement but do not block the core replacement release.
- `DROP`: remove intentionally with a documented data disposition.

| ID | Legacy capability | Disposition | Salvage decision | First release scope | Go target | Required acceptance evidence |
| --- | --- | --- | --- | --- | --- | --- |
| MW-DOC-001 | TXT/Markdown/text-PDF upload | REDESIGN | CORE_REBUILD_FROM_ZERO | CORE | upload, blob, ingestion | duplicate, cancel, crash, retry, source hash and locator E2E |
| MW-DOC-002 | Document lifecycle | REDESIGN | CORE_REBUILD_FROM_ZERO | CORE | document/generation | ACTIVE/TRASHED/PURGE_PENDING/PURGED state model tests |
| MW-DOC-003 | Reindex | REDESIGN | LATER_FROM_ZERO | LATER | generation coordinator | old generation remains queryable; per-document atomic activation |
| MW-DOC-004 | Deterministic chunks and FTS indexing | REBUILD | CORE_REBUILD_FROM_ZERO | CORE | chunking/SQLite FTS pipeline | deterministic golden chunks, transactional FTS refresh and lifecycle filtering |
| MW-COL-001 | Collections | KEEP_SEMANTICS | CORE_KEEP_SEMANTICS | CORE | collection membership | true many-to-many scope; empty scope never becomes global |
| MW-RET-001 | Simple lexical retrieval | REDESIGN | CORE_REBUILD_FROM_ZERO | CORE | controlled SQLite FTS retrieval | active lifecycle and collection isolation; stable source chunk IDs |
| MW-RET-002 | Hybrid/RRF/rerank/expansion | REDESIGN | LATER_FROM_ZERO | LATER | retrieval stages | one executable path; backend/stage conformance and quality gates |
| MW-EMB-001 | Embeddings and vector retrieval | REBUILD | LATER_FROM_ZERO | LATER | future embedding/vector slice | no first-release schema/provider/route; later corpus, capacity and backend conformance |
| MW-RAG-001 | Ask | KEEP_SEMANTICS | CORE_REBUILD_FROM_ZERO | CORE | answer run | scoped lexical answer records final source chunk IDs and handles provider failure honestly |
| MW-RAG-002 | Citations | REDESIGN | CORE_REBUILD_FROM_ZERO | CORE | answer citations | every citation resolves to a source chunk supplied for that answer |
| MW-RAG-003 | Honest refusal and labeling | REDESIGN | CORE_REBUILD_FROM_ZERO | CORE | answer policy | no-hit and structurally invalid citation cases refuse or carry an explicit limitation label |
| MW-CON-001 | Conversation history | KEEP_SEMANTICS | CORE_REBUILD_FROM_ZERO | CORE | conversation/message/answer | immutable answer provenance and purge behavior |
| MW-PRO-001 | Model providers | REDESIGN | CORE_REBUILD_FROM_ZERO | CORE | versioned providers/egress | real adapter probe, chosen-config source binding, SSRF/key isolation |
| MW-JOB-001 | Ordinary Task/outbox | DROP | DROP | DROP | unified durable job | no separate task executor or RabbitMQ path |
| MW-AGT-001 | Agent tasks | REDESIGN | LATER_FROM_ZERO | LATER | agent run on unified job | atomic steps, read-only tools first, budgets, cancellation, crash recovery |
| MW-AGT-002 | Agent profiles | REDESIGN | LATER_FROM_ZERO | LATER | agent/profile | versioned profile updates, deletion lineage and memory-scope enforcement |
| MW-MEM-001 | Memory | REDESIGN | LATER_FROM_ZERO | LATER | explicit memory | opt-in auto-write, source/scope/retention, shared context budget |
| MW-BAT-001 | Batch import | REDESIGN | LATER_FROM_ZERO | LATER | group of ingestion jobs | bounded concurrency, partial failure, cancellation and staging retention |
| MW-NOT-001 | User notifications | REDESIGN | LATER_FROM_ZERO | LATER | durable notification projection | job outcome/recovery notifications are deduplicated, attributable and operable in the UI |
| MW-EVL-001 | Evaluation | REDESIGN | LATER_FROM_ZERO | LATER | versioned evaluation | fixed corpus/dataset fingerprint and same retrieval snapshot as generation |
| MW-VEC-001 | Qdrant | DEFER | LATER_FROM_ZERO | LATER | optional backend | full generation/scope/delete/audit conformance before release |
| MW-HLT-001 | Vector/index health | REDESIGN | LATER_FROM_ZERO | LATER | diagnostics/repair | real backend capabilities, dry-run, backup and repair receipt |
| MW-TRS-001 | Trash/restore | KEEP_SEMANTICS | CORE_KEEP_SEMANTICS | CORE | retention coordinator | immediate retrieval exclusion and reversible restore |
| MW-TRS-002 | Permanent delete | REDESIGN | CORE_REBUILD_FROM_ZERO | CORE | document deletion | delete known in-scope DB/FTS rows and reference-aware blobs; failures never report success |
| MW-BKP-001 | Backup/restore | REDESIGN | CORE_REBUILD_FROM_ZERO | CORE | backup coordinator | DB snapshot + blob manifest; restore on a clean machine |
| MW-UI-001 | Static management UI | REDESIGN | CORE_REBUILD_FROM_ZERO | CORE | embedded web UI | API contract, browser security, accessibility and recovery workflows |
| MW-CFG-001 | Legacy properties, profiles and secrets | REDESIGN | CORE_REBUILD_FROM_ZERO | CORE | platform/config and credential store | explicit field translation, unknown-key rejection and no plaintext secret export |
| MW-RUN-001 | Local launch, shutdown and environment scripts | REDESIGN | CORE_REBUILD_FROM_ZERO | CORE | single-process runtime and signed Windows package | one executable owns the Vault; install/start/stop/uninstall pass clean-machine tests |
| MW-STO-001 | Storage summary and cache controls | REDESIGN | LATER_FROM_ZERO | LATER | storage diagnostics and repair plans | authoritative/derived bytes are distinguished and destructive actions emit verified receipts |
| MW-CCH-001 | Embedding and retrieval caches | REBUILD | LATER_FROM_ZERO | LATER | derived cache/index stores | canonical inputs and fingerprints are migrated; cache rows are regenerated, never trusted as canonical |
| MW-TSK-001 | Ordinary task history, prompts and results | ARCHIVE_ONLY | ARCHIVE_ONLY | ARCHIVE | neutral migration archive | history is readable and checksummed but can never re-enter the Go Job state machine |
| MW-DEV-001 | Development-only task mutation/dispatch endpoints | DROP | DROP | DROP | none | production build exposes no dev mutation or dispatch surface |
| MW-RPT-001 | Legacy evaluation and benchmark report files | ARCHIVE_ONLY | ARCHIVE_ONLY | ARCHIVE | migration report attachments | existing reports are checksummed and labeled legacy; new gates generate versioned Go reports |
| MW-MIG-001 | Legacy data | REDESIGN | CORE_REBUILD_FROM_ZERO | CORE | neutral migration package | read-only export, idempotent import, quarantine and hash report |
| MW-INF-001 | MySQL | DROP | DROP | DROP | SQLite | offline temporary-database integration tests and integrity checks |
| MW-INF-002 | RabbitMQ | DROP | DROP | DROP | SQLite job queue | atomic claim, one lease token, bounded retry, cancellation and restart recovery |
| MW-INF-003 | Python workers | DROP | DROP | DROP | Go orchestration + Ollama | no Python runtime dependency; parser isolation remains possible |
| MW-INF-004 | Java, Spring and Maven runtime/build stack | DROP | DROP | DROP | standalone Go module and release toolchain | packaged runtime contains no JVM, Maven or Spring dependency |
| MW-INF-005 | Optional local Ollama runtime | KEEP_SEMANTICS | CORE_REBUILD_FROM_ZERO | CORE | versioned Ollama provider adapter | offline capability probe, explicit model selection and no mandatory cloud dependency |
| MW-INF-006 | Legacy Docker Compose development stack | DROP | DROP | DROP | optional test fixtures only | production install does not require Docker; fixture use is isolated from user Vaults |

Every row must eventually link to an ADR, implementation package, migration rule,
tests, user documentation, and release gate. A row cannot be closed solely because
an endpoint or page exists.

`Salvage decision` governs product scope, not source-code reuse. In particular,
`CORE_REBUILD_FROM_ZERO` means the need belongs in the first release while the
legacy implementation is rejected as a design input. `LATER_FROM_ZERO` is absent
from the first release and may be introduced only by a later, independently
accepted vertical slice. See [`legacy-salvage-review.md`](./legacy-salvage-review.md).
