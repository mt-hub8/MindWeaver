# Rewrite acceptance ledger

Status values are `NOT_IMPLEMENTED`, `IMPLEMENTED`, `PASS`, and `BLOCKED`.
`PASS` requires a committed automated test or a versioned release artifact plus
the exact command/report location. Manual inspection alone cannot close a row.
Only `Phase=CORE` rows are release blockers. `LATER` rows preserve
traceability without expanding the first Go release.

Current CORE reconciliation: `21 PASS`, `8 IMPLEMENTED`, `4 BLOCKED`, and
`4 NOT_IMPLEMENTED`. All nine `LATER` rows remain `NOT_IMPLEMENTED` and are
not part of the first Go release.

| ID | Required outcome | Phase | Core gate | Current status | Required evidence |
| --- | --- | --- | --- | --- | --- |
| ARC-001 | `v2/` builds from an empty standalone checkout | CORE | YES | PASS | `v2/scripts/verify-standalone.*`; repeat in release CI |
| ARC-002 | No Go production path imports or executes legacy Java/Python | CORE | YES | PASS | `v2/openapi/v1/contract_test.go`; `v2/qualification/production/dual_pe_test.go`; exact two-PE/negative-surface audit |
| ARC-003 | Exactly one accepted implementation exists for each CORE capability; later paths are absent | CORE | YES | NOT_IMPLEMENTED | package graph and core feature closure matrix |
| RUN-001 | One process owns one canonical Vault with an OS lock | CORE | YES | PASS | `v2/internal/vault/vault_test.go`; `v2/cmd/mindweaver/main_test.go`; two-process kill/reopen tests |
| RUN-002 | Startup migration and reconciliation finish before ordinary writes | CORE | YES | IMPLEMENTED | startup ordering plus 20-run Answer/ingestion interruption qualification pass; full schema-migration checkpoint forced-exit matrix remains; `docs/rewrite/evidence/runtime-interruption-current.md` |
| RUN-003 | Bounded shutdown checkpoints work; crash recovery does not rely on shutdown | CORE | YES | PASS | app-owned bounded drain plus stable incomplete-shutdown classification; real Answer/ingestion/DOC/Blob kill-reopen and backup residue recovery; `docs/rewrite/evidence/run003-shutdown-recovery-4a338dc.md` |
| RUN-004 | Loopback ephemeral listener never becomes LAN reachable | CORE | YES | PASS | `v2/internal/localhttp/server_test.go`; IPv4 bind/Host/Origin/DNS-rebinding suite |
| CFG-001 | Versioned core config rejects unknown/duplicate fields and unsafe Vault paths | CORE | YES | PASS | `v2/platform/config/config_test.go`; Vault path and clean-start integration tests |
| DB-001 | SQLite schema migrates from every declared supported version | CORE | YES | PASS | `v2/internal/store/sqlite/migration_compat_test.go`; v1-v7 fixtures/checksum/future-version rejection; v7 removes the never-used `settings` table without changing historical checksums |
| DB-002 | Single-writer WAL, busy timeout, disk-full, corruption and integrity modes are measured | CORE | YES | IMPLEMENTED | `v2/internal/store/sqlite/qualification_fault_test.go`; randomized/24-48h machine stress remains |
| BLOB-001 | Bounded import never exposes partial bytes; publication, dedupe and cleanup are retry-safe within documented OS guarantees | CORE | YES | IMPLEMENTED | publication/orphan/cancellation evidence is in `docs/rewrite/evidence/blob001-publication-cancellation-0dff1d4.md`; the frozen matrix, crash results, K05 feasibility decision and deterministic staging Write/Sync failures are in `docs/rewrite/evidence/blob001-persistence-checkpoint-matrix-b99fb42.md`; backup/deletion exclusion is in `docs/rewrite/evidence/blob-backup-interaction.md`; registered-volume evidence is in `docs/rewrite/evidence/windows-registered-volume-boundary-57e293b.md`; K01B2/K05/K08, physical ENOSPC and Linux no-replace certification prevent PASS |
| JOB-001 | A core background operation cannot be executed concurrently by two workers | CORE | YES | PASS | `v2/internal/store/sqlite/jobs_test.go`; concurrent claim and stale-owner fencing tests |
| JOB-002 | Core retries stop at a configured limit; cancel and final failure stay visible | CORE | YES | PASS | ingestion retry/cancel/terminal/restart product tests |
| PROG-001 | Document ingestion and Ask progress never reports success before durable completion | CORE | YES | IMPLEMENTED | `v2/cmd/mindweaver/progress_recovery_test.go`; `v2/qualification/knowledge/doc001_remaining_qualification_test.go`; real-process HTTP and durable recovery pass, but external real-browser forced-exit qualification remains blocked; `docs/rewrite/evidence/prog001-forced-progress-74d56a4.md` |
| BKP-001 | Backup manifest comes from its own DB snapshot and every blob verifies | CORE | YES | BLOCKED | `docs/rewrite/evidence/live-backup-create.md`; registered-volume admission closes the retained Windows remap boundary; packaged clean-machine restore/reopen plus installer, upgrade/rollback, uninstall, and release-attestation evidence remain open |
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
| RAG-003 | No-hit or structurally invalid citation cases refuse or show an explicit limitation label | CORE | YES | IMPLEMENTED | backend refusal and honest static UI wording pass; real-browser label evidence remains |
| CON-001 | Conversation/messages preserve immutable provenance and version conflicts | CORE | YES | IMPLEMENTED | durable provenance/retry/restart tests pass; true two-tab browser conflict remains |
| MEM-001 | A future Memory slice is explicit, scoped, budgeted, attributable and purgeable | LATER | NO | NOT_IMPLEMENTED | injection/budget/lineage tests before promotion |
| AGT-001 | A future Agent slice uses unified Job, read-only tools, budgets and durable receipts | LATER | NO | NOT_IMPLEMENTED | cancel/crash/duplicate step E2E before promotion |
| BAT-001 | A future Batch slice has bounded concurrency, per-item idempotency and staging cleanup | LATER | NO | NOT_IMPLEMENTED | zero-slot/race/partial-failure tests before promotion |
| EVL-001 | A future Evaluation slice freezes dataset, pipeline, model and result fingerprints | LATER | NO | NOT_IMPLEMENTED | reproducibility report before promotion |
| API-001 | OpenAPI is complete for CORE routes and generated/implemented behavior matches it | CORE | YES | PASS | `v2/openapi/v1/{contract_test.go,production_surface_test.go}`; state-discriminated responses and exact production closure |
| API-002 | Retried upload/Ask does not duplicate core work and stale mutable-root revisions are rejected | CORE | YES | IMPLEMENTED | backend retry/revision and immutable WebUI attempts pass; true multi-tab browser conflict remains |
| SEC-001 | One-use bootstrap, session, CSRF, Host, Origin and CSP pass attack suite | CORE | YES | BLOCKED | raw HTTP suite passes; approved browser/driver and real Windows process sandbox are unavailable |
| SEC-002 | Logs/events/artifacts exclude secrets, prompts and source content by policy | CORE | YES | IMPLEMENTED | focused canary/redaction suites pass; release-wide evidence scan remains |
| UI-001 | Enabled CORE first-run, offline/no-model, progress, recovery and diagnostics are operable | CORE | YES | BLOCKED | browser qualification contract is fail-closed; approved artifact and real process harness remain |
| UI-002 | Enabled CORE workflows pass keyboard, focus, scaling, high contrast and Chinese input checks | CORE | YES | BLOCKED | 13 browser scenarios remain `NOT_RUN` until approved artifact/process harness exists |
| REL-001 | Race, fuzz, fault, performance and soak gates pass for CORE scope | CORE | YES | NOT_IMPLEMENTED | core release qualification bundle |
| REL-002 | Windows package is signed, offline installable, upgradable and uninstallable | CORE | YES | NOT_IMPLEMENTED | clean-machine matrix, SBOM and signatures |
| CUT-001 | Go is the sole product writer; installation creates a fresh Vault and exposes no Java/MySQL data-import path | CORE | YES | PASS | fresh-first-run, negative migration surface, exact two-PE and 890-file legacy DROP audit |
| CUT-002 | Extracted repository builds/packages with no parent dependency | CORE | YES | NOT_IMPLEMENTED | clean-clone release CI |

`ARC-001` records the current walking-skeleton evidence only. It must be rerun
after every integration and again from the extracted repository before `CUT-002`
can pass.
