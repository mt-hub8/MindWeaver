# CORE acceptance boundary reconciliation on `43d2448`

Status: **RUN-002, PROG-001 and BKP-001 PASS**

This deletion-oriented review is bound to committed product baseline
`43d2448c37b5df1d1a2c78a0eec3ee36f7f5a032`. Later integrated changes before
this report are documentation, evidence, inventory, and validator changes only;
the production and test witnesses named below are byte-identical. The review
removes three duplicated or unobservable blockers. It adds no production hook,
VFS, journal, schema, API, or alternate state machine.

## RUN-002: startup ordering

Every migration's SQL and matching ledger row execute in one
`BEGIN IMMEDIATE` transaction. A crash during SQLite `COMMIT` can expose only
the old complete transaction or the new complete transaction. On restart, the
old endpoint is retried and the new endpoint is recognized by its exact ledger
name/checksum. There is no schema-without-ledger or ledger-without-schema third
product state to qualify, so a COMMIT hook or custom VFS would add test
machinery without closing a product gap.

The executable closure is:

- v0 through v7 upgrade, checksum/gap/future-version rejection, and failed
  migration rollback in `internal/store/sqlite`;
- the v1 through v6, before-first-pending-transaction and
  after-all-migrations-before-listener forced-exit matrix in
  `qualification/runtime`;
- WAL forced-exit recovery; and
- App startup ordering: lock, staging cleanup, Store open/migrate, job recovery,
  Blob/lifecycle cleanup and Answer reconciliation all precede route/listener
  and worker availability.

The focused matrix, migration/rollback/WAL tests, startup/RAG reconciliation
tests, and related vet set passed. Physical torn-write or power-loss behavior
remains a DB-002/platform qualification boundary; it is not counted again in
RUN-002.

## PROG-001: durable progress

Ingestion can expose `succeeded` only after one SQLite transaction writes
chunks, activates the revision, updates the document, and finalizes the job.
Failure rolls the transaction back. Ask begins as durable `pending`; HTTP maps
that state to 202. It can return completed only after the terminal Answer,
sources, citations, and completion time have committed and been read back.

Real serve-process tests authenticate through the bootstrap/session/CSRF
boundary, kill an Ask after the provider receives it, reopen the Vault, and
observe the same Answer as `failed/OUTCOME_UNCERTAIN` without a provider replay.
The ingestion child is killed at a durable worker checkpoint; the next owner
recovers it without duplicate document, Blob, chunk, or FTS effects. The
embedded UI contract labels upload as indexing until the durable job succeeds
and consumes only backend terminal Answer state.

The serve-process Ask test, DOC forced-termination test, ingestion transaction
and stale-worker tests, RAG completion/reopen tests, App HTTP product test, and
runtime interruption qualification all passed repeatedly. Browser rendering,
reconnect, focus, accessibility, and IME behavior remain only UI-001/UI-002
qualification; they do not duplicate the durable-progress outcome here.

## BKP-001: snapshot and Blob verification

`backup.Coordinator.Create` pins Blob deletion, uses SQLite's Online Backup API
to create its own database snapshot, derives schema and ordered Blob references
only from that snapshot, and verifies every referenced object before and after
manifest commitment. Standalone verification copies and qualifies the database,
rehashes every Blob, re-enumerates snapshot references, and compares the exact
ordered manifest and tree. Standalone restore without an active or control
Vault imports content-addressed objects without merge/overwrite, reopens the
restored Vault/SQLite database, and repeats the reference/tree checks.

Concurrent-write snapshot, purge/sweep exclusion, standalone verify,
no-active-Vault restore/reopen, real recovery CLI, forced publication residue,
post-rename cancellation, live HTTP create/replay, and App shutdown-drain tests
all passed. These tests close the stated backup data invariant. Packaging that
same flow in a signed installer, N-1/N-2 upgrade/rollback, uninstall, and an
attested external clean VM remains REL-002; it is not counted again in BKP-001.

## Reproduction

The exact automated witnesses include:

```text
go test ./qualification/runtime -run '^TestRUN002SchemaMigrationForcedExitMatrix$'
go test ./internal/store/sqlite -run 'Test(EveryDeclaredSchemaVersionUpgradesToCurrent|FailedMigrationRollsBackSchemaAndLedger|MigrationLedgerRejectsChecksumGapAndNewerVersion|QualificationForcedExitRecovery)'
go test ./cmd/mindweaver -run '^TestServeProcessForcedAskRestartPollsDurableFailureWithoutProviderReplay$'
go test ./qualification/knowledge -run '^TestDOC001ForcedTerminationRecovery$'
go test ./internal/backup -run 'Test(BackupUnderWritesRestoresSnapshotAndEveryBlob|CreatePinExcludesPurgeAndSweepThroughCleanRestore|VerifyStandaloneNeedsNoLiveCoordinator|CleanMachineRecoveryRestoresWithoutControlVaultAndReopens)'
go test ./cmd/mindweaver ./internal/app ./internal/backup
go vet ./qualification/runtime ./qualification/knowledge ./cmd/mindweaver ./internal/app ./internal/backup ./internal/store/sqlite
```

The focused current-main reviews reran their critical tests up to ten times;
the complete backup package also passed. Historical evidence files retain their
original baseline and measurements. Their older `IMPLEMENTED` or `BLOCKED`
classification is superseded only for these three acceptance rows, not erased.
