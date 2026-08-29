# RUN-003 bounded shutdown and crash recovery

Status: **PASS for RUN-003 only**

- Audited production baseline:
  `4a338dc1ffd22e139703b5f5e4a907bb733e2fbe`
- Recorded: 2026-08-27 (Asia/Shanghai)
- This slice changes acceptance evidence only. It changes no production code,
  schema, protocol, or test.

## Accepted claim

Shutdown quiesces HTTP, backup, RAG, and ingestion admission before draining all
four under one application-owned eight-second deadline. A drain failure returns
the stable, content-free `ErrShutdownIncomplete`, keeps SQLite and the Vault lock
open for process reclamation, and lets the CLI exit after one bounded call. A
caller's shorter deadline cannot replace the application's final shutdown
result. Successful shutdown drains accepted backup work and RAG terminal writes
before closing SQLite and releasing the Vault lock.

Recovery correctness does not require that shutdown ran. Real process
termination and reopen tests cover pending Answer reconciliation without
provider replay, fenced ingestion recovery without duplicate chunks or FTS
rows, document-worker recovery, and Blob states K01A, K02, K03, K04, K06, and
K09. K07 has no persisted claim and has the same restart input as K04. Backup
forced-exit receipts, restore residue, and post-rename publication uncertainty
remain explicit and recoverable. In each application-owned case, startup
cleanup and reconciliation run before ordinary routes or workers become
available.

## Deliberate exclusions

This decision does not promote `RUN-002`, `BLOB-001`, or `BKP-001`. In
particular, K01B2 (termination inside file or directory sync), K05 (an exactly
observed SQLite commit-in-flight outcome), and K08 (unlink before directory
sync), along with physical ENOSPC and Linux no-replace certification, remain
Blob qualification gaps. They are persistence-window certification work, not
application shutdown checkpoints, and do not show that recovery depends on a
graceful shutdown. The broader data-safety fault matrix therefore remains open
without weakening the narrower RUN-003 result.

The historical 20-run report at
`docs/rewrite/evidence/runtime-interruption-current.md` remains unchanged and
retains its original baseline and limits. This current-main evidence combines
that real Answer/ingestion result with the subsequently committed shutdown,
Blob, document, and backup evidence.

## Independent reproduction on the audited baseline

Using the repository-selected Go 1.27 toolchain, these commands passed in a
clean Windows worktree:

```text
go test ./internal/app -run '^(TestAppShutdownCancelsAndDrainsAcceptedBackupBeforeClosingStore|TestAppShutdownNeverReleaseFaultsAreBoundedAndKeepLowerResourcesOpen|TestAppShutdownFirstCallerTimeoutDoesNotFixFinalResult|TestRAGShutdownWaitsForCanceledAskTerminalWrite)$' -count=10
PASS (32.344s)

go test ./cmd/mindweaver -run '^TestServeShutdownIsBoundedAndCallsAppOnce$' -count=10
PASS (2.752s)

go test ./qualification/runtime -run '^TestRuntimeInterruptionQualification$' -count=1
PASS (22.757s)

go test ./qualification/knowledge -run '^(TestDOC001ForcedTerminationRecovery|TestBLOB001IncompleteStagingRecoversAfterForcedTermination|TestBLOB001DurableStagingBeforeCandidateRecoversAfterForcedTermination|TestBLOB001DurableCandidateBeforeRenameRecoversAfterForcedTermination|TestBLOB001PublishedOrphanRecoversAfterForcedTermination|TestBLOB001ReferenceCommitResponseReplayAfterForcedTermination|TestBLOB001DurableDeleteBeforeCandidateResolveRecoversAfterForcedTermination)$' -count=1
PASS (17.624s)

go test ./internal/backup -run '^(TestForcedExitResiduesAreListedAndExplicitlyRecovered|TestStartupRestoreResidueRecoveryRecoversOnlyExactForcedExitResidues|TestRestoreBlobImportCancellationAfterRenameRetainsExactRecoverableResidue|TestConfirmPublishedBackupRecoversForcedExitAfterReceiptRemoval)$' -count=1
PASS (5.018s)
```
