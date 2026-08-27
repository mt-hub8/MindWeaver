# BLOB-001 open-window deletion review

Status: **NO PRODUCTION CHANGE — BLOB-001 remains IMPLEMENTED**

- Audited baseline:
  `1a41c27a76a74e8710587779610a4a7672370fa7`
- Recorded: 2026-08-27 (Asia/Shanghai)
- This review supersedes only the current acceptance interpretation of K01B2,
  K05, and K08. It does not rewrite the frozen matrix at
  `docs/rewrite/evidence/blob001-persistence-checkpoint-matrix-b99fb42.md`.

## Deletion ruling

None of the three open labels describes a third product-visible state or a
missing recovery action. Adding a production hook, journal, or state machine
would only make a nearby user-space point observable; it would not reproduce
termination inside an OS sync call or SQLite's commit decision.

### K01B2: staging sync outcome unobserved

`Prepare` has not renamed the private staging file, so no final-tree Blob is
visible. Termination inside file or staging-directory sync can leave private
staging present, absent, incomplete, or complete without proving physical
durability. Startup does not interpret or publish that state: while holding the
Vault lock it removes owned inactive staging and syncs the staging directory
before ordinary routes or workers exist. K01A and K02 prove the adjacent
incomplete and completed staging recovery paths. A callback can pause only
before or after `Sync`; an inside-syscall or power-loss result requires an OS,
VFS, or hardware qualification harness, not a CORE production seam.

### K05: document-reference commit outcome unobserved

The document, revision, job, ingestion receipt, and Blob candidate removal are
one `BEGIN IMMEDIATE` transaction. After process termination SQLite exposes
either the K04 state (published object plus candidate, no graph) or the K06
state (complete graph and no candidate). K04 startup sweep and K06 exact replay
both converge to one graph and one object. A callback before `Commit` proves
only the old state; one after `Commit` proves only the new state. A generic
transaction hook or a second commit journal would add a competing source of
truth without qualifying SQLite's internal commit instruction.

### K08: unlink outcome not yet directory-synced

The durable Blob candidate is removed only after `DeletionGuard.Delete` returns
from parent-directory sync. A terminated process therefore leaves the
candidate regardless of whether the object name is present after restart. If
present, the K07/K04 reference-aware path retries deletion. If absent, the
missing-object path syncs the parent again before candidate resolution, the
same convergence proven at K09. Production retains the candidate whenever
`Delete` returns an error, while the deterministic sync-failure test proves the
missing-object directory sync is repaired on retry. A pause between `Remove`
and `Sync` would not prove inside-sync or power-loss durability and would
duplicate the already-covered endpoint states.

## Remaining acceptance boundary

K01B2, K05, and K08 remain honest descriptions of unobserved syscall/commit
windows, but they are not independent CORE product blockers and do not justify
production-only observability. BLOB-001 remains `IMPLEMENTED`, not `PASS`,
because clean-machine physical ENOSPC/sync-durability behavior and Linux atomic
no-replace publication still lack supported-platform certification. This
review does not qualify sudden power loss, damaged media, or the Linux path.

## Reproduction

The following commands passed in a clean Windows worktree on the audited
baseline with the repository-selected Go 1.27 toolchain:

```text
go test ./internal/blob -run '^(TestPrepareStagingWriteAndSyncFailuresFailClosed|TestDeletionRetrySyncsAlreadyMissingObject)$' -count=10
PASS (1.079s)

go test ./internal/store/sqlite -run '^(TestCreateDocumentUploadResolvesCandidateOnCreateAndExactReplayOnly|TestCreateDocumentUploadFailureKeepsCandidateAndRollsBackGraph)$' -count=10
PASS (1.029s)

go test ./internal/lifecycle -run '^(TestPreparedUploadBoundariesConvergeThroughCandidateSweep|TestUploadConflictLeavesRecoverableCandidateWithoutTouchingReferencedBlob|TestConcurrentSameBlobUploadsLeaveReferencesWithoutCandidate)$' -count=10
PASS (2.852s)

go test ./qualification/knowledge -run '^(TestBLOB001IncompleteStagingRecoversAfterForcedTermination|TestBLOB001DurableStagingBeforeCandidateRecoversAfterForcedTermination|TestBLOB001PublishedOrphanRecoversAfterForcedTermination|TestBLOB001ReferenceCommitResponseReplayAfterForcedTermination|TestBLOB001DurableDeleteBeforeCandidateResolveRecoversAfterForcedTermination)$' -count=1
PASS (12.207s)
```
