# Rewrite acceptance ledger

Status values are `NOT_IMPLEMENTED`, `IMPLEMENTED`, `PASS`, and `BLOCKED`.
`PASS` requires a committed automated test or a versioned release artifact plus
the exact command/report location. Manual inspection alone cannot close a row.

| ID | Required outcome | Current status | Required evidence |
| --- | --- | --- | --- |
| ARC-001 | `v2/` builds from an empty standalone checkout | PASS | `v2/scripts/verify-standalone.*`; repeat in release CI |
| ARC-002 | No Go production path imports or executes legacy Java/Python | IMPLEMENTED | dependency and packaged-runtime audit |
| ARC-003 | One accepted implementation per retained capability | NOT_IMPLEMENTED | package graph and feature closure matrix |
| RUN-001 | One process owns one canonical Vault with an OS lock | NOT_IMPLEMENTED | two-process Windows integration test |
| RUN-002 | Startup migration and reconciliation finish before ordinary writes | NOT_IMPLEMENTED | staged runtime state test and forced exits |
| RUN-003 | Bounded shutdown checkpoints work; crash recovery does not rely on shutdown | NOT_IMPLEMENTED | checkpoint kill matrix |
| RUN-004 | Loopback ephemeral listener never becomes LAN reachable | NOT_IMPLEMENTED | bind/Host/DNS rebinding tests |
| CFG-001 | Versioned config rejects unknown/duplicate fields and unsafe paths | IMPLEMENTED | duplicate-key and canonical Vault tests still required |
| ID-001 | IDs are monotonic UUIDv7 under concurrency and clock rollback | NOT_IMPLEMENTED | deterministic race/rollback/entropy tests |
| DB-001 | SQLite schema migrates from every supported N-1/N-2 version | NOT_IMPLEMENTED | migration fixtures and compatibility report |
| DB-002 | Single-writer WAL, busy timeout, disk-full, corruption and integrity modes are measured | NOT_IMPLEMENTED | SQLite feasibility benchmark/fault report |
| BLOB-001 | Intent/staging/hash/sync/rename/reference protocol is crash safe | NOT_IMPLEMENTED | fault after every persistence boundary |
| JOB-001 | Atomic claim, writer epoch and fencing reject every late worker | NOT_IMPLEMENTED | concurrent claim/takeover integration test |
| JOB-002 | Retry is bounded; cancellation, poison work and `NEEDS_ATTENTION` close | NOT_IMPLEMENTED | state-machine and scheduler tests |
| EVT-001 | State and event commit together; SSE is global, resumable and bounded | NOT_IMPLEMENTED | rollback/cursor/overrun tests |
| BKP-001 | Backup manifest comes from its own DB snapshot and every blob verifies | NOT_IMPLEMENTED | concurrent-write backup/restore rehearsal |
| PUR-001 | Purge plans lineage-complete deletion and signs a verified local receipt | NOT_IMPLEMENTED | injected deletion failure and residual scan |
| DOC-001 | TXT/Markdown/PDF upload is idempotent, bounded and crash recoverable | NOT_IMPLEMENTED | duplicate/cancel/timeout/crash E2E |
| DOC-002 | Trash excludes immediately; restore is reversible; purge is not overstated | NOT_IMPLEMENTED | lifecycle and retention E2E |
| GEN-001 | Reindex keeps old generation live and atomically activates only its document | NOT_IMPLEMENTED | two-document same-generation regression test |
| COL-001 | Membership is true many-to-many and empty scope stays empty | NOT_IMPLEMENTED | multi-collection and empty-scope Golden tests |
| RET-001 | Retrieval returns only active documents and active validated generations | NOT_IMPLEMENTED | lifecycle/generation leakage corpus |
| RET-002 | Lexical/vector/fusion/rerank stages preserve score provenance and fingerprint | NOT_IMPLEMENTED | deterministic conformance suite |
| RET-003 | Chinese lexical choice and vector capacity tiers are measured | NOT_IMPLEMENTED | representative quality/performance report |
| PRV-001 | Provider config/capability/credential binding is immutable and versioned | NOT_IMPLEMENTED | update/probe/credential rotation integration test |
| PRV-002 | Egress dials only approved IPs and reauthorizes redirect without ambient proxy | NOT_IMPLEMENTED | hostile DNS/redirect/proxy server suite |
| INV-001 | Invocation closes PREPARED/SENT/RECEIVED/COMMITTED crash windows | NOT_IMPLEMENTED | kill matrix with durable result artifacts |
| INV-002 | Unsafe SENT calls become `OUTCOME_UNCERTAIN` and never auto-replay | NOT_IMPLEMENTED | duplicate-cost recovery test |
| RAG-001 | Search, Ask, Agent and Evaluation share one RetrievalSnapshot | NOT_IMPLEMENTED | cross-entry conformance test |
| RAG-002 | Every citation belongs to exact same-answer model context | NOT_IMPLEMENTED | foreign/unseen/stale citation rejection tests |
| RAG-003 | Strict verification publishes a supported answer or controlled refusal | NOT_IMPLEMENTED | adversarial grounding corpus |
| CON-001 | Conversation/messages preserve immutable provenance and version conflicts | NOT_IMPLEMENTED | multi-tab and retry E2E |
| MEM-001 | Memory is explicit, scoped, budgeted, attributable and purgeable | NOT_IMPLEMENTED | injection/budget/lineage tests |
| AGT-001 | Agent steps use unified Job, read-only tools, budgets and durable receipts | NOT_IMPLEMENTED | cancel/crash/duplicate step E2E |
| BAT-001 | Batch ingest has bounded concurrency, per-item idempotency and staging cleanup | NOT_IMPLEMENTED | zero-slot/race/partial-failure tests |
| EVL-001 | Evaluation freezes dataset, pipeline, model and result fingerprints | NOT_IMPLEMENTED | reproducibility report |
| API-001 | OpenAPI is complete, generated/implemented behavior matches it | IMPLEMENTED | path coverage is currently partial |
| API-002 | Idempotency and If-Match prevent duplicate work and lost updates | NOT_IMPLEMENTED | replay/key-conflict/multi-tab tests |
| SEC-001 | One-use bootstrap, session, CSRF, Host, Origin and CSP pass attack suite | NOT_IMPLEMENTED | browser and raw HTTP security suite |
| SEC-002 | Logs/events/artifacts exclude secrets, prompts and source content by policy | NOT_IMPLEMENTED | canary leakage scan |
| UI-001 | First run, offline/no-model, progress, recovery and diagnostics are operable | NOT_IMPLEMENTED | browser E2E screenshots and assertions |
| UI-002 | Keyboard, focus, scaling, high contrast and Chinese input pass | NOT_IMPLEMENTED | accessibility acceptance report |
| MIG-001 | Java exporter is read-only and emits a neutral checksummed package | NOT_IMPLEMENTED | exporter tests against versioned fixtures |
| MIG-002 | Go import is idempotent, rebuilds derivatives and quarantines ambiguity | NOT_IMPLEMENTED | import-twice and quarantine report |
| MIG-003 | Largest real Vault verifies hashes, relations, lifecycle and exceptions | NOT_IMPLEMENTED | sanitized signed verification report |
| REL-001 | Race, fuzz, fault, performance and soak gates pass | NOT_IMPLEMENTED | release qualification bundle |
| REL-002 | Windows package is signed, offline installable, upgradable and uninstallable | NOT_IMPLEMENTED | clean-machine matrix, SBOM and signatures |
| CUT-001 | Go is sole writer; Java is frozen read-only with supported migration exit | NOT_IMPLEMENTED | final cutover record |
| CUT-002 | Extracted repository builds/packages with no parent dependency | NOT_IMPLEMENTED | clean-clone release CI |

`ARC-001` records the current walking-skeleton evidence only. It must be rerun
after every integration and again from the extracted repository before `CUT-002`
can pass.
