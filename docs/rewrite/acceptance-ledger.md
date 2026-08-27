# Rewrite acceptance ledger

Status values are `NOT_IMPLEMENTED`, `IMPLEMENTED`, `PASS`, and `BLOCKED`.
`PASS` requires a committed automated test or a versioned release artifact plus
the exact command/report location. Manual inspection alone cannot close a row.
Only `Phase=CORE` rows are release blockers. `LATER` rows preserve
traceability without expanding the first Go release.

Current CORE reconciliation: `30 PASS`, `1 IMPLEMENTED`, `3 BLOCKED`, and
`2 NOT_IMPLEMENTED`. All nine `LATER` rows remain `NOT_IMPLEMENTED` and are
not part of the first Go release.

| ID | Required outcome | Phase | Core gate | Current status | Required evidence |
| --- | --- | --- | --- | --- | --- |
| ARC-001 | `v2/` builds from an empty standalone checkout | CORE | YES | PASS | `v2/scripts/verify-standalone.*`; repeat in release CI |
| ARC-002 | No Go production path imports or executes legacy Java/Python | CORE | YES | PASS | `v2/openapi/v1/contract_test.go`; `v2/qualification/production/dual_pe_test.go`; exact two-PE/negative-surface audit |
| RUN-001 | One process owns one canonical Vault with an OS lock | CORE | YES | PASS | `v2/internal/vault/vault_test.go`; `v2/cmd/mindweaver/main_test.go`; two-process kill/reopen tests |
| RUN-002 | Startup migration and reconciliation finish before ordinary writes | CORE | YES | PASS | v0-v7 atomic migration/rollback plus v1-v6 forced-exit and reconciliation-before-listener tests; COMMIT exposes only complete old/new endpoints; `docs/rewrite/evidence/core-acceptance-boundary-reconciliation-43d2448.md` |
| RUN-003 | Bounded shutdown checkpoints work; crash recovery does not rely on shutdown | CORE | YES | PASS | app-owned bounded drain plus stable incomplete-shutdown classification; real Answer/ingestion/DOC/Blob kill-reopen and backup residue recovery; `docs/rewrite/evidence/run003-shutdown-recovery-4a338dc.md` |
| RUN-004 | Loopback ephemeral listener never becomes LAN reachable | CORE | YES | PASS | `v2/internal/localhttp/server_test.go`; IPv4 bind/Host/Origin/DNS-rebinding suite |
| CFG-001 | Versioned core config rejects unknown/duplicate fields and unsafe Vault paths | CORE | YES | PASS | `v2/platform/config/config_test.go`; Vault path and clean-start integration tests |
| DB-001 | SQLite schema migrates from every declared supported version | CORE | YES | PASS | `v2/internal/store/sqlite/migration_compat_test.go`; v1-v7 fixtures/checksum/future-version rejection; v7 removes the never-used `settings` table without changing historical checksums |
| DB-002 | Single-writer WAL, busy timeout, logical `SQLITE_FULL` transaction atomicity, corruption and integrity modes are measured | CORE | YES | PASS | real SQLite WAL/BUSY, deterministic `SQLITE_FULL`, corruption/integrity, forced-exit and bounded fixed-seed stress evidence; numeric BUSY/LOCKED maps to stable retryable HTTP and durable ingestion retry semantics; `docs/rewrite/evidence/{db002-bounded-wal-stress-7b75f8f.md,db002-product-contention-d825e4e.md}`; physical-media qualification remains only under REL-001 |
| BLOB-001 | Bounded import never exposes partial bytes; publication, dedupe and cleanup are retry-safe within documented OS guarantees | CORE | YES | PASS | publication/orphan/cancellation and the frozen crash matrix remain executable; real Windows no-replace, exact concurrent dedupe and the machine/platform boundary are closed in `docs/rewrite/evidence/blob001-windows-closure-768d66e.md`; physical media/power-loss campaigns remain under REL-001 and Linux remains unsupported |
| JOB-001 | A core background operation cannot be executed concurrently by two workers | CORE | YES | PASS | `v2/internal/store/sqlite/jobs_test.go`; concurrent claim and stale-owner fencing tests |
| JOB-002 | Core retries stop at a configured limit; cancel and final failure stay visible | CORE | YES | PASS | ingestion retry/cancel/terminal/restart product tests |
| PROG-001 | Document ingestion and Ask progress never reports success before durable completion | CORE | YES | PASS | real-process HTTP Ask/ingestion forced-exit, durable terminal-state and exact-replay tests; browser presentation stays under UI gates; `docs/rewrite/evidence/core-acceptance-boundary-reconciliation-43d2448.md` |
| BKP-001 | Backup manifest comes from its own DB snapshot and every blob verifies | CORE | YES | PASS | concurrent snapshot, exact manifest/blob verification, no-active-Vault standalone restore/reopen and forced-residue tests; installer lifecycle stays under REL-002; `docs/rewrite/evidence/core-acceptance-boundary-reconciliation-43d2448.md` |
| PUR-001 | Explicit delete removes known in-scope DB/FTS rows and unshared blobs; failure never reports success | CORE | YES | PASS | `v2/internal/lifecycle/service_test.go`; SQLite/filesystem failure, shared-blob and retry tests |
| DOC-001 | TXT/Markdown/PDF upload is idempotent, bounded and crash recoverable | CORE | YES | PASS | `v2/qualification/knowledge/{production_ingestion_lifecycle_test.go,doc001_remaining_qualification_test.go}`; `docs/rewrite/evidence/{production-ingestion-lifecycle-fda7c63.md,doc001-final-bb176c8.md}` |
| DOC-002 | Trash excludes immediately; restore is reversible; purge is not overstated | CORE | YES | PASS | lifecycle trash/restore/purge projection and restart E2E |
| GEN-001 | A future reindex slice keeps old generation live and activates only its document | LATER | NO | NOT_IMPLEMENTED | two-document same-generation regression test before promotion |
| COL-001 | Membership is true many-to-many and empty scope stays empty | CORE | YES | PASS | SQLite/API M:N, revision, restart and explicit-empty-scope tests |
| RET-001 | Literal continuous-phrase SQLite FTS returns only active documents in the requested collection scope | CORE | YES | PASS | `v2/qualification/knowledge/core_keyword_boundary_test.go`; `v2/testdata/qualification/knowledge/core-keyword-boundary.8aadf60.v2.json`; Chinese/English, deterministic order, two-code-point refusal, lifecycle/empty/cross-collection corpus |
| RET-002 | Future natural-question query understanding/fusion/rerank/expansion preserves scope, lifecycle and score provenance | LATER | NO | NOT_IMPLEMENTED | representative deterministic conformance suite and accepted budgets before promotion; all existing candidates remain `Selection=NONE` |
| RET-003 | Future Chinese/English natural-question retrieval has accepted recall, ranking, FDR, 100k capacity/latency/disk, incremental-maintenance and restart budgets | LATER | NO | NOT_IMPLEMENTED | `v2/qualification/knowledge/core_keyword_boundary_test.go`; `v2/testdata/qualification/knowledge/core-keyword-boundary.8aadf60.v2.json`; prior bounded-instr, FTS5 hybrid and relational term-index candidates remain rejected and must not enter production |
| EMB-001 | A future embedding/vector slice proves model, capacity, lifecycle and backend conformance | LATER | NO | NOT_IMPLEMENTED | representative corpus and backend suite before promotion |
| PRV-001 | Each message records the chosen versioned, non-secret loopback Ollama configuration; no credential surface exists | CORE | YES | PASS | configure/probe/invoke/restart source-binding integration tests |
| PRV-002 | Ollama dials only a fixed literal loopback address and rejects DNS names, ambient proxy, and redirect authority changes | CORE | YES | PASS | `v2/internal/ollama/client_test.go`; DNS/proxy/redirect/literal-loopback suite |
| INV-001 | A provider call has bounded timeout/cancel behavior and ends in one durable user-visible success or failure | CORE | YES | PASS | timeout/cancel/truncated-body/restart tests; post-write ambiguity persists as `OUTCOME_UNCERTAIN` |
| INV-002 | A future enhanced invocation protocol classifies ambiguous transmitted requests without unsafe automatic replay | LATER | NO | NOT_IMPLEMENTED | crash-window and duplicate-cost suite before promotion |
| RAG-001 | Keyword/continuous-phrase-driven Ask uses scoped SQLite FTS and stores the final source chunk IDs with the answer | CORE | YES | PASS | literal-phrase/no-expansion, scope, persistence, restart and source-change tests; natural-question retrieval is LATER |
| RAG-002 | Every citation resolves to a source chunk supplied for that answer | CORE | YES | PASS | foreign, missing, changed and malformed citation rejection tests |
| RAG-003 | No-hit or structurally invalid citation cases refuse or show an explicit limitation label | CORE | YES | PASS | backend/API refusal and honest static UI contract tests; `docs/rewrite/evidence/rag-con-api-current-main-43d2448.md`; real-browser presentation remains only under `UI-001`/`UI-002` |
| CON-001 | Conversation/messages preserve immutable provenance and version conflicts | CORE | YES | PASS | durable provenance, exact replay, revision conflict and restart tests; `docs/rewrite/evidence/rag-con-api-current-main-43d2448.md`; real two-tab presentation remains only under `UI-001` |
| MEM-001 | A future Memory slice is explicit, scoped, budgeted, attributable and purgeable | LATER | NO | NOT_IMPLEMENTED | injection/budget/lineage tests before promotion |
| AGT-001 | A future Agent slice uses unified Job, read-only tools, budgets and durable receipts | LATER | NO | NOT_IMPLEMENTED | cancel/crash/duplicate step E2E before promotion |
| BAT-001 | A future Batch slice has bounded concurrency, per-item idempotency and staging cleanup | LATER | NO | NOT_IMPLEMENTED | zero-slot/race/partial-failure tests before promotion |
| EVL-001 | A future Evaluation slice freezes dataset, pipeline, model and result fingerprints | LATER | NO | NOT_IMPLEMENTED | reproducibility report before promotion |
| API-001 | OpenAPI is complete for CORE routes and generated/implemented behavior matches it | CORE | YES | PASS | `v2/openapi/v1/{contract_test.go,production_surface_test.go}`; state-discriminated responses and exact production closure |
| API-002 | Retried upload/Ask does not duplicate core work and stale mutable-root revisions are rejected | CORE | YES | PASS | backend retry/revision plus immutable WebUI attempt contracts; `docs/rewrite/evidence/rag-con-api-current-main-43d2448.md`; real multi-tab presentation remains only under `UI-001` |
| SEC-001 | One-use bootstrap, session, CSRF, Host, Origin and CSP pass attack suite | CORE | YES | BLOCKED | raw HTTP and Windows Job/ACL sandbox-primitives pass; repository-approved browser/driver, family launch/network/profile policy and real-run CSP evidence remain; `docs/rewrite/evidence/browser-qualification-current-c384a40.md` |
| SEC-002 | Logs/events/artifacts exclude secrets, prompts and source content by policy | CORE | YES | IMPLEMENTED | focused canary/redaction suites pass; release-wide evidence scan remains |
| UI-001 | Enabled CORE first-run, offline/no-model, progress, recovery and diagnostics are operable | CORE | YES | BLOCKED | browser qualification contract is fail-closed; repository-approved artifacts, family launch policy, fresh-Vault/fake-Ollama orchestration and real-run evidence remain; `docs/rewrite/evidence/browser-qualification-current-c384a40.md` |
| UI-002 | Enabled CORE workflows pass keyboard, focus, scaling, high contrast and Chinese input checks | CORE | YES | BLOCKED | all required browser scenarios remain `NOT_RUN` until the approved artifact/launch/orchestration prerequisites exist; `docs/rewrite/evidence/browser-qualification-current-c384a40.md` |
| REL-001 | Race, fuzz, fault, performance and soak gates pass for CORE scope | CORE | YES | NOT_IMPLEMENTED | current developer signals and release-scale gaps are bounded in `docs/rewrite/evidence/rel001-current-main-37ca406.md`; approved race, physical-fault, 24-48h soak, 1,000 random-kill, 100k/SLO and accepted-duration/corpus multi-component operation-sequence gates remain |
| REL-002 | Windows package is signed, offline installable, upgradable and uninstallable | CORE | YES | NOT_IMPLEMENTED | clean-machine matrix, SBOM and signatures |
| CUT-001 | Go is the sole product writer; installation creates a fresh Vault and exposes no Java/MySQL data-import path | CORE | YES | PASS | fresh-first-run, negative migration surface, exact two-PE and 890-file legacy DROP audit |
| CUT-002 | Extracted repository builds/packages with no parent dependency | CORE | YES | PASS | `v2/scripts/{ci,verify-standalone}.*`; `v2/qualification/{offlinevendor,production}`; current tracked-only extraction and exact dual-PE evidence in `docs/rewrite/evidence/dual-pe-production-closure.md` |

`ARC-001` records the current walking-skeleton evidence only. The final
integration reran it both in the source checkout and from the tracked-only
extracted repository to close `CUT-002`; future production changes must repeat
both gates before retaining that status.
