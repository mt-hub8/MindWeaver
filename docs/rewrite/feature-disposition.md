# Legacy feature disposition

Statuses:

- `KEEP_SEMANTICS`: preserve the user-observable contract.
- `REDESIGN`: preserve the user need but replace the implementation and failure semantics.
- `REBUILD`: migrate canonical input and regenerate derived data.
- `ARCHIVE_ONLY`: retain readable history without entering new execution state machines.
- `DEFER`: retain the requirement but do not block the core replacement release.
- `DROP`: remove intentionally with a documented data disposition.

| ID | Legacy capability | Disposition | Go target | Required acceptance evidence |
| --- | --- | --- | --- | --- |
| MW-DOC-001 | TXT/Markdown/text-PDF upload | REDESIGN | upload, blob, ingestion | duplicate, cancel, crash, retry, source hash and locator E2E |
| MW-DOC-002 | Document lifecycle | REDESIGN | document/generation | ACTIVE/TRASHED/PURGE_PENDING/PURGED state model tests |
| MW-DOC-003 | Reindex | REDESIGN | generation coordinator | old generation remains queryable; per-document atomic activation |
| MW-DOC-004 | Chunks and embeddings | REBUILD | chunk/index pipeline | deterministic golden chunks and compatible fingerprint validation |
| MW-COL-001 | Collections | KEEP_SEMANTICS | collection membership | true many-to-many scope; empty scope never becomes global |
| MW-RET-001 | Keyword/vector retrieval | REDESIGN | unified retrieval engine | active generation and lifecycle isolation; score provenance |
| MW-RET-002 | Hybrid/RRF/rerank/expansion | REDESIGN | retrieval stages | one executable path; backend/stage conformance and quality gates |
| MW-RAG-001 | Ask | KEEP_SEMANTICS | answer run | scoped answer with durable snapshot, cancellation and provider failure handling |
| MW-RAG-002 | Citations | REDESIGN | AnswerContext/citation | every citation belongs to exact model context; no stale/unseen citation |
| MW-RAG-003 | Grounding/refusal | REDESIGN | verification policy | strict mode never publishes an answer that policy rejected |
| MW-CON-001 | Conversation history | KEEP_SEMANTICS | conversation/message/answer | immutable answer provenance and purge behavior |
| MW-PRO-001 | Model providers | REDESIGN | versioned providers/egress | capability probe, immutable invocation snapshot, SSRF/key isolation |
| MW-JOB-001 | Ordinary Task/outbox | DROP | unified durable job | no separate task executor or RabbitMQ path |
| MW-AGT-001 | Agent tasks | REDESIGN | agent run on unified job | atomic steps, read-only tools first, budgets, cancellation, crash recovery |
| MW-MEM-001 | Memory | REDESIGN | explicit memory | opt-in auto-write, source/scope/retention, shared context budget |
| MW-BAT-001 | Batch import | REDESIGN | group of ingestion jobs | bounded concurrency, partial failure, cancellation and staging retention |
| MW-EVL-001 | Evaluation | REDESIGN | versioned evaluation | fixed corpus/dataset fingerprint and same retrieval snapshot as generation |
| MW-VEC-001 | Qdrant | DEFER | optional backend | full generation/scope/delete/audit conformance before release |
| MW-HLT-001 | Vector/index health | REDESIGN | diagnostics/repair | real backend capabilities, dry-run, backup and repair receipt |
| MW-TRS-001 | Trash/restore | KEEP_SEMANTICS | retention coordinator | immediate retrieval exclusion and reversible restore |
| MW-TRS-002 | Permanent purge | REDESIGN | purge plan/receipt | lineage-complete verified deletion; failures never report success |
| MW-BKP-001 | Backup/restore | REDESIGN | backup coordinator | DB snapshot + blob manifest; restore on a clean machine |
| MW-UI-001 | Static management UI | REDESIGN | embedded web UI | API contract, browser security, accessibility and recovery workflows |
| MW-MIG-001 | Legacy data | REDESIGN | neutral migration package | read-only export, idempotent import, quarantine and hash report |
| MW-INF-001 | MySQL | DROP | SQLite | offline temporary-database integration tests and integrity checks |
| MW-INF-002 | RabbitMQ | DROP | SQLite job queue | atomic claim, fencing, bounded retry, outcome-uncertain handling |
| MW-INF-003 | Python workers | DROP | Go orchestration + Ollama | no Python runtime dependency; parser isolation remains possible |

Every row must eventually link to an ADR, implementation package, migration rule,
tests, user documentation, and release gate. A row cannot be closed solely because
an endpoint or page exists.
