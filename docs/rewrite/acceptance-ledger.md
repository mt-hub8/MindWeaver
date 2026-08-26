# Rewrite acceptance ledger

Status values are `NOT_IMPLEMENTED`, `IMPLEMENTED`, `PASS`, and `BLOCKED`.
`PASS` requires a committed automated test or a versioned release artifact plus
the exact command/report location. Manual inspection alone cannot close a row.
Only `Phase=CORE` rows are release blockers. `LATER` rows preserve
traceability without expanding the first Go release.

| ID | Required outcome | Phase | Core gate | Current status | Required evidence |
| --- | --- | --- | --- | --- | --- |
| ARC-001 | `v2/` builds from an empty standalone checkout | CORE | YES | PASS | `v2/scripts/verify-standalone.*`; repeat in release CI |
| ARC-002 | No Go production path imports or executes legacy Java/Python | CORE | YES | IMPLEMENTED | dependency and packaged-runtime audit |
| ARC-003 | Exactly one accepted implementation exists for each CORE capability; later paths are absent | CORE | YES | NOT_IMPLEMENTED | package graph and core feature closure matrix |
| RUN-001 | One process owns one canonical Vault with an OS lock | CORE | YES | NOT_IMPLEMENTED | two-process Windows integration test |
| RUN-002 | Startup migration and reconciliation finish before ordinary writes | CORE | YES | NOT_IMPLEMENTED | staged runtime state test and forced exits |
| RUN-003 | Bounded shutdown checkpoints work; crash recovery does not rely on shutdown | CORE | YES | NOT_IMPLEMENTED | checkpoint kill matrix |
| RUN-004 | Loopback ephemeral listener never becomes LAN reachable | CORE | YES | NOT_IMPLEMENTED | bind/Host/DNS rebinding tests |
| CFG-001 | Versioned core config rejects unknown/duplicate fields and unsafe Vault paths | CORE | YES | NOT_IMPLEMENTED | focused parser/path tests and clean-start integration test |
| DB-001 | SQLite schema migrates from every declared supported version | CORE | YES | NOT_IMPLEMENTED | migration fixtures and compatibility report |
| DB-002 | Single-writer WAL, busy timeout, disk-full, corruption and integrity modes are measured | CORE | YES | NOT_IMPLEMENTED | SQLite feasibility benchmark/fault report |
| BLOB-001 | Bounded import never exposes partial bytes; publication, dedupe and cleanup are retry-safe within documented OS guarantees | CORE | YES | NOT_IMPLEMENTED | fault after every persistence boundary and Windows durability tests |
| JOB-001 | A core background operation cannot be executed concurrently by two workers | CORE | YES | NOT_IMPLEMENTED | duplicate-delivery and lease-takeover integration test |
| JOB-002 | Core retries stop at a configured limit; cancel and final failure stay visible | CORE | YES | NOT_IMPLEMENTED | ingestion retry/cancel/restart tests |
| PROG-001 | Document ingestion and Ask progress never reports success before durable completion | CORE | YES | NOT_IMPLEMENTED | reconnect/poll and forced-exit workflow tests |
| BKP-001 | Backup manifest comes from its own DB snapshot and every blob verifies | CORE | YES | BLOCKED | `docs/rewrite/evidence/live-backup-create.md`; retained Windows kernel-path capability and clean-machine release rehearsal remain open |
| PUR-001 | Explicit delete removes known in-scope DB/FTS rows and unshared blobs; failure never reports success | CORE | YES | NOT_IMPLEMENTED | SQLite/filesystem failure, shared-blob and retry tests |
| DOC-001 | TXT/Markdown/PDF upload is idempotent, bounded and crash recoverable | CORE | YES | NOT_IMPLEMENTED | Partial production qualification: `v2/qualification/knowledge/production_ingestion_lifecycle_test.go`; `docs/rewrite/evidence/production-ingestion-lifecycle-fda7c63.md`; forced-exit and bounded-input matrix remains |
| DOC-002 | Trash excludes immediately; restore is reversible; purge is not overstated | CORE | YES | NOT_IMPLEMENTED | lifecycle and retention E2E |
| GEN-001 | A future reindex slice keeps old generation live and activates only its document | LATER | NO | NOT_IMPLEMENTED | two-document same-generation regression test before promotion |
| COL-001 | Membership is true many-to-many and empty scope stays empty | CORE | YES | NOT_IMPLEMENTED | SQLite/API multi-collection and empty-scope integration tests |
| RET-001 | Simple SQLite FTS returns only active documents in the requested collection scope | CORE | YES | NOT_IMPLEMENTED | lifecycle, empty-scope and cross-collection leakage corpus |
| RET-002 | Future fusion/rerank/expansion stages preserve score provenance and fingerprint | LATER | NO | NOT_IMPLEMENTED | deterministic conformance suite before promotion |
| RET-003 | Core Chinese SQLite FTS tokenization and bounded result quality are measured | CORE | YES | BLOCKED | `v2/qualification/knowledge/core_keyword_boundary_test.go`; `v2/testdata/qualification/knowledge/core-keyword-boundary.d61a.v1.json`; `docs/rewrite/evidence/core-keyword-boundary-d61a.md` |
| EMB-001 | A future embedding/vector slice proves model, capacity, lifecycle and backend conformance | LATER | NO | NOT_IMPLEMENTED | representative corpus and backend suite before promotion |
| PRV-001 | Each message records the chosen versioned, non-secret loopback Ollama configuration; no credential surface exists | CORE | YES | NOT_IMPLEMENTED | configure/probe/invoke/restart source-binding integration test |
| PRV-002 | Ollama dials only a fixed literal loopback address and rejects DNS names, ambient proxy, and redirect authority changes | CORE | YES | NOT_IMPLEMENTED | DNS-name rejection, resolver canary, redirect and proxy suite |
| INV-001 | A provider call has bounded timeout/cancel behavior and ends in one durable user-visible success or failure | CORE | YES | NOT_IMPLEMENTED | local fake-provider timeout, cancel, restart and result tests |
| INV-002 | A future enhanced invocation protocol classifies ambiguous transmitted requests without unsafe automatic replay | LATER | NO | NOT_IMPLEMENTED | crash-window and duplicate-cost suite before promotion |
| RAG-001 | Ask uses scoped SQLite FTS and stores the final source chunk IDs with the answer | CORE | YES | NOT_IMPLEMENTED | lexical Ask persistence and source-change tests |
| RAG-002 | Every citation resolves to a source chunk supplied for that answer | CORE | YES | NOT_IMPLEMENTED | foreign, missing and malformed citation rejection tests |
| RAG-003 | No-hit or structurally invalid citation cases refuse or show an explicit limitation label | CORE | YES | NOT_IMPLEMENTED | no-hit, malformed-response and browser-label tests |
| CON-001 | Conversation/messages preserve immutable provenance and version conflicts | CORE | YES | NOT_IMPLEMENTED | multi-tab and retry E2E |
| MEM-001 | A future Memory slice is explicit, scoped, budgeted, attributable and purgeable | LATER | NO | NOT_IMPLEMENTED | injection/budget/lineage tests before promotion |
| AGT-001 | A future Agent slice uses unified Job, read-only tools, budgets and durable receipts | LATER | NO | NOT_IMPLEMENTED | cancel/crash/duplicate step E2E before promotion |
| BAT-001 | A future Batch slice has bounded concurrency, per-item idempotency and staging cleanup | LATER | NO | NOT_IMPLEMENTED | zero-slot/race/partial-failure tests before promotion |
| EVL-001 | A future Evaluation slice freezes dataset, pipeline, model and result fingerprints | LATER | NO | NOT_IMPLEMENTED | reproducibility report before promotion |
| API-001 | OpenAPI is complete for CORE routes and generated/implemented behavior matches it | CORE | YES | NOT_IMPLEMENTED | core path coverage; later routes must be absent |
| API-002 | Retried upload/Ask does not duplicate core work and stale mutable-root revisions are rejected | CORE | YES | NOT_IMPLEMENTED | focused retry and multi-tab conflict tests |
| SEC-001 | One-use bootstrap, session, CSRF, Host, Origin and CSP pass attack suite | CORE | YES | NOT_IMPLEMENTED | browser and raw HTTP security suite |
| SEC-002 | Logs/events/artifacts exclude secrets, prompts and source content by policy | CORE | YES | NOT_IMPLEMENTED | canary leakage scan |
| UI-001 | Enabled CORE first-run, offline/no-model, progress, recovery and diagnostics are operable | CORE | YES | NOT_IMPLEMENTED | browser E2E screenshots and assertions |
| UI-002 | Enabled CORE workflows pass keyboard, focus, scaling, high contrast and Chinese input checks | CORE | YES | NOT_IMPLEMENTED | accessibility acceptance report |
| REL-001 | Race, fuzz, fault, performance and soak gates pass for CORE scope | CORE | YES | NOT_IMPLEMENTED | core release qualification bundle |
| REL-002 | Windows package is signed, offline installable, upgradable and uninstallable | CORE | YES | NOT_IMPLEMENTED | clean-machine matrix, SBOM and signatures |
| CUT-001 | Go is the sole product writer; installation creates a fresh Vault and exposes no Java/MySQL data-import path | CORE | YES | NOT_IMPLEMENTED | production dependency/surface audit and clean first-run record |
| CUT-002 | Extracted repository builds/packages with no parent dependency | CORE | YES | NOT_IMPLEMENTED | clean-clone release CI |

`ARC-001` records the current walking-skeleton evidence only. It must be rerun
after every integration and again from the extracted repository before `CUT-002`
can pass.
