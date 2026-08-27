# DB-002 product contention closure

Status: **PASS for the DB-002 acceptance outcome**

- Production and executable-test commit:
  `d825e4e1b8421de3cacb68bb6d17f6578e965f34`
- Prior bounded stress evidence:
  `docs/rewrite/evidence/db002-bounded-wal-stress-7b75f8f.md`
- Recorded: 2026-08-27 (Asia/Shanghai)

This evidence closes the one product-semantic gap retained by the earlier
stress report. It does not change SQLite's transaction model, add an automatic
transaction-callback retry loop, add a writer queue, or add a second journal.

## Closed product behavior

`sqlite.IsRetryableContention` recognizes only numeric SQLite `BUSY` and
`LOCKED` primary codes. Wrapped extended results such as `BUSY_RECOVERY`,
`BUSY_SNAPSHOT`, `BUSY_TIMEOUT`, `LOCKED_SHAREDCACHE`, and `LOCKED_VTAB` retain
those primary codes and are therefore included. `SQLITE_FULL`, `INTERRUPT`, and
error text that merely says "busy" or "locked" are not misclassified.

At the HTTP product boundary, contention now becomes a bounded, content-free
`503 SERVICE_UNAVAILABLE` Problem with `retryable=true` and the stable action
`retry_later`. Documents, collections, purge status, purge listings, and upload
all use the same classifier. The test crosses a real literal-loopback TCP
listener and the production bootstrap/session/CSRF boundary. Its injected
`BUSY_TIMEOUT` is deliberately synthetic: it proves the transport mapping,
while the SQLite qualification tests independently produce real numeric
contention from retained transactions.

If an ingestion worker encounters numeric contention after claiming a job, the
existing lease-token-protected `FailOrRetry` transition releases the lease and
keeps the job queued with stable code `DATABASE_BUSY`. The existing attempt
limit, cancellation precedence, and stale-worker fencing remain authoritative.
The worker does not replay an arbitrary transaction callback.

## Measured storage modes

The existing executable suite already measures the remaining DB-002 wording:

- WAL readers retain an old snapshot while a second writer reaches real numeric
  `BUSY` or `LOCKED`, followed by exact whole-operation retry;
- deterministic `SQLITE_FULL` is produced with SQLite `max_page_count`, and the
  rejected transaction leaves no partial rows;
- corrupt database headers fail closed without overwrite;
- structural integrity and FTS5 external-content divergence are checked
  separately;
- uncommitted WAL state is killed in a child process and reopened with one valid
  committed endpoint;
- fixed-seed multi-writer invocation pressure, retained WAL readers, rollbacks,
  checkpoints, close/reopen, and exact model fingerprints are bounded and
  repeatable.

DB-002 now names the deterministic SQLite `SQLITE_FULL` transaction behavior
above directly; it does not claim physical media exhaustion qualification.

## Boundary owned by REL-001

Physical ENOSPC, filesystem quota, controller cache, torn-sector or sudden
power-loss behavior; an accepted 1,000-random-kill campaign; a 24–48 hour
supported-machine soak; race qualification; and accepted 100k performance/RSS
budgets remain release-scale work under REL-001. They are not duplicated as
DB-002 blockers.

Likewise, there is no claim that a test stopped inside SQLite's COMMIT
instruction. The deterministic process-termination model observed only the
complete old or complete new durable transaction endpoints; existing
idempotency, CAS, reopen, and replay tests bind both. A timing hook or second
application journal would add complexity without proving the VFS or hardware
window.

## Reproduction

From `v2/` with the repository-selected Go 1.27.0 toolchain and offline vendor
environment:

```powershell
go test ./internal/store/sqlite -run '^Test(IsRetryableContentionUsesNumericPrimaryAndExtendedCodes|QualificationWALReadersAndBusyWriter|QualificationSQLiteFullRollsBackWholeTransition|QualificationIntegrityDetectsCorruptDatabaseAndFTSDivergence|QualificationForcedExitRecovery|QualificationBoundedSeededWALStress)$' -count=5
go test ./internal/app -run '^TestSQLiteContentionIsAContentFreeRetryableHTTPFailure$' -count=10
go test ./internal/workbench -run '^TestInterruptedClaimRetriesUnlessUserCancellationWins$' -count=10
go test ./openapi/v1 -run '^(TestSQLiteBackedReadOperationsDeclareRetryableContention|TestEmbeddedContractMatchesProduction)$' -count=1
go test ./internal/store/sqlite ./internal/app ./internal/workbench ./openapi/v1 -count=1
go vet ./internal/store/sqlite ./internal/app ./internal/workbench ./openapi/v1
```

The focused tests, the four complete packages, and vet passed during the
independent review. The production slice was reviewed with P0=0 and P1=0.
