# DB-002 bounded deterministic WAL stress evidence

- Recorded: 2026-08-27 (Asia/Shanghai)
- Acceptance row: `DB-002`
- Base revision: `7b75f8f4735f7ddd54b619dd0dbe73f253fa3a80`
- Executable evidence:
  `v2/internal/store/sqlite/qualification_stress_test.go`
- Executable-evidence SHA-256:
  `758c223c5e8535672aeb8b2c1f7d0d6ef01261b51c50ad20024f4cd46e2ac258`
- Ledger effect: none; `DB-002` remains `IMPLEMENTED`, not `PASS`

This report adds a bounded, fixed-seed developer stress slice to the existing
fault qualifications in `qualification_fault_test.go`. It changes no production
code, migration, migration checksum, database schema, or acceptance-ledger row.
The baseline migration tree object remained
`25f89df1fcab9e855350ba8cc592f104bad4cda9`.

## Environment

| Item | Recorded value |
| --- | --- |
| Operating system | Microsoft Windows 11 家庭版 中文版, 10.0.26200 build 26200, x64 |
| CPU | Intel(R) Core(TM) Ultra X7 358H |
| PowerShell | 7.6.4 |
| Go | go1.27.0 windows/amd64 |
| `CGO_ENABLED` | `0` |
| SQLite Go driver | `github.com/ncruces/go-sqlite3 v0.35.3` |

No Linux execution is claimed by this Windows report. The test uses portable Go,
`database/sql`, and SQLite pragmas, but Linux remains a separate runner result.

## Frozen model and seeds

Each scenario creates test-local STRICT tables inside a real production-opened
SQLite file: two accounts with a constant total balance, an exact signed-transfer
operation ledger, and a committed revision counter. The schema exists only in the
temporary qualification database; embedded production migrations are unchanged.

The plan is frozen by these seeds and dimensions:

| Seed | Retained WAL readers | Writer goroutines | Rounds | Planned commits | Injected rollbacks | BUSY retry commit | Final commits |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `0x0db002001` | 1 | 4 | 12 | 40 | 8 | 1 | 41 |
| `0x0db0020a5` | 4 | 4 | 12 | 38 | 10 | 1 | 39 |
| `0x0db002f17` | 8 | 4 | 12 | 36 | 12 | 1 | 37 |

Every failed assertion identifies the seed, round, reader, or exact operation
ID. The seed reproduces the operation plan, not the Go scheduler interleaving.
Transfers commute, begin with enough balance to avoid plan-dependent constraint
failures, and have a deterministic final state independent of scheduler order.
An interleaving-only failure therefore still requires the recorded test output
and repeated execution; this slice does not claim a deterministic scheduler.

## Invariants exercised

### Default-busy-timeout concurrent invocation pressure

The randomized writer rounds use the production `5s` busy timeout and one real
`Store`. The pool capacity is deliberately expanded to retain 1, 4, or 8
physical WAL readers while four writer goroutines exercise the production
`withTx` path; it is not the production default connection count. Every reader
and writer must report ready before the round releases a single start gate. A
scheduler yield occurs after each debit to widen the half-transition window,
but readers must still see the exact pre-stress snapshot.

The ready gate proves concurrent invocation attempts, not that SQLite admitted
multiple write transactions simultaneously or that the Go scheduler overlapped
every callback. SQLite's single-writer serialization is the behavior under
test; the separate retained-holder probe below is the direct contention/BUSY
evidence.

After every round the test independently verifies:

- both exact balances and their constant total;
- the exact sorted committed operation-ID and signed-delta fingerprint;
- operation-ledger count equals the committed revision;
- injected rollback operations are absent, including their balance changes.

The three default-timeout scenarios passed ten complete repetitions. No default
configuration `BUSY`, partial transition, integrity error, or acknowledged model
loss was observed.

### Deliberate bounded BUSY and exact complete-operation retry

The deliberate contention probe is separated from the default-timeout pressure.
It changes only one retained contender connection to `busy_timeout=80`, verifies
a numeric `BUSY`/`LOCKED` result no earlier than the configured bound (allowing
25ms scheduler/clock tolerance), and restores that connection to `5000ms` before
returning it to the pool.

The lock holder writes a complete test transition and then rolls it back. The
contender calls the same complete-operation function with the same operation ID;
its first `BEGIN IMMEDIATE` fails with BUSY before any callback write, and its
retry after holder rollback commits exactly once. The exact model proves that
the holder transition did not leak and only the retried operation is present.

Focused-run observations were 81.4004ms, 80.7506ms, and 81.1228ms for the three
seeds. These are observations, not latency SLOs.

### Cancellation, rollback, and same-connection reuse

On a retained physical connection, an immediate transaction first writes a
complete transition and then runs a finite 10,000,000-row recursive SQLite query
under a 10ms context deadline. The test accepts only context cancellation or the
numeric SQLite `INTERRUPT` classification, rolls the transaction back, checks the
exact unchanged model through that same connection, and then successfully runs
`SELECT 40 + 2` on it.

Focused-run observations were 10.0783ms, 10.1993ms, and 10.3988ms. The 15-second
scenario context is a cooperative bound, not an independent watchdog around the
synchronous driver call. The recorded commands use Go's independent
`-timeout=30s` process watchdog, and the recursive query has finite work. A
broken cancellation implementation may therefore fail the command by timeout;
this test does not claim that such a connection would remain reusable.

### WAL checkpoint, integrity, and close/reopen

While the reader transactions retain their original snapshots, a passive
checkpoint must report uncheckpointed WAL frames (`log > checkpointed`). After
all reader transactions commit and their physical connections close, a truncate
checkpoint must report `0/0/0`. The exact model, SQLite structural check,
foreign-key check, FTS external-content check, and canonical application check
then pass before close and again after normal production reopen.

The existing `TestQualificationForcedExitRecovery` remains the process-exit/WAL
recovery evidence. This bounded test's close/reopen is deliberately a clean
durability boundary, not another power-loss claim.

## Exact commands and results

Commands ran from the independent clean worktree's `v2` directory with:

```powershell
$mwGo = 'C:\Users\24281\AppData\Local\MindWeaver\toolchains\go1.27.0\bin\go.exe'
& $mwGo test -timeout=30s -run '^TestQualificationBoundedSeededWALStress$' -count=1 -v ./internal/store/sqlite
& $mwGo test -timeout=30s -run '^TestQualificationBoundedSeededWALStress$' -count=10 ./internal/store/sqlite
& $mwGo test -timeout=30s -run '^TestQualification' -count=5 ./internal/store/sqlite
& $mwGo test -timeout=30s -count=1 ./internal/store/sqlite
& $mwGo vet ./internal/store/sqlite
```

| Command | Result | Go package duration | Measured wall time |
| --- | --- | ---: | ---: |
| New test, verbose, count 1 | PASS | 0.837s | 1.494s |
| New test, count 10 | PASS | 6.917s | 8.253s |
| All `TestQualification*`, count 5 | PASS | 5.931s | 6.590s |
| Full SQLite package, count 1 | PASS | 3.891s | 4.561s |
| SQLite package vet | PASS | n/a | 0.428s |

## P0/P1 and claim boundary

- P0: none found. Every completed run retained the exact committed model and
  passed integrity before and after reopen.
- P1: the accepted ADR 0010 target is not fully implemented. The committed 80ms
  probe directly exercises the same driver `BEGIN IMMEDIATE` path and returns a
  numeric `BUSY`; source audit confirms `Store.withTx` preserves that numeric
  cause but production has no single-writer lane or complete-transaction
  retry/normalization layer. If such an error reaches the HTTP boundary,
  `classifyError` safely hides raw driver text but maps it to non-retryable
  `INTERNAL` rather than a stable retryable code. Production-default 5s stress did
  not reproduce BUSY, so this report records the gap without changing production.

This evidence does **not** replace or claim:

- physical disk-full, filesystem quota, short-write, controller-cache, torn
  sector, or sudden-power-loss behavior;
- commit-adjacent `IOERR` outcome reconciliation;
- 1,000 random process kills around acknowledged/unacknowledged operation IDs;
- 24-48 hours of WAL, checkpoint, online backup, Defender/EDR, sleep/resume, and
  random-kill stress on supported Windows 10 and Windows 11 machines;
- online backup under sustained writes or 100 restore/integrity rehearsals;
- race/fuzz, Linux execution, or release-machine performance and RSS thresholds.

Accordingly, this is incremental DB-002 evidence only. It must not raise
`DB-002` from `IMPLEMENTED` to `PASS`.
