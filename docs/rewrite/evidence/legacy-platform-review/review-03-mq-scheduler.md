# Review 03: MQ and schedulers

Status: complete at `0df22ddaf02c64bf73a7df12cd5fea6b52632c73`.

All RabbitMQ producers, consumers, message DTOs, schedulers, and associated
tests are `DROP`. The Go desktop product has one local SQLite work authority;
it does not retain a broker, a distributed delivery protocol, or a second
durable task truth.

## Frozen review set

`review-03-files.csv` is the authoritative per-file ledger with baseline,
path, SHA-256, byte count, line count, full `1-N` range, scope class, and
disposition.

| Scope | Kind | Files | Lines | Reviewed range |
| --- | --- | ---: | ---: | --- |
| primary | `mq` and `scheduler` production Java | 15 | 435 | every file, line 1 through EOF |
| primary | tests physically under `scheduler` | 2 | 169 | every file, line 1 through EOF |
| cross-reference | tests in other packages importing MQ/scheduler types | 8 | 910 | every file, line 1 through EOF |
| **Reviewed evidence total** |  | **25** | **1,514** | **complete** |

The primary exact-set is 17 files / 604 lines. The eight cross-reference tests
also appeared as data-model references in review 02; they are listed here for
MQ reasoning but are not counted again in the final primary total.

The SHA-256 of `review-03-files.csv` is
`0260a268b24ed5eeb1da30279f5287b01202f07e2d5ceed54912ca90593a2bb5`.

## File-level disposition

- Eleven MQ classes/interfaces: `DROP`.
- Four scheduler classes: `DROP`.
- Two primary scheduler tests and eight cross-reference tests: `DROP` as
  implementation and contract evidence.
- Agent tasks, reindex, batch orchestration, Rabbit exchanges/queues/DLQ, and
  external vector delivery remain outside CORE.

The sole narrow semantic marker is that external I/O must not execute inside
a SQL transaction. That rule is already enforced by accepted Go boundaries;
it does not retain `TaskOutboxDispatcherScheduler` or RabbitMQ.

## P0 findings

1. **Direct Rabbit publication occurs before the owning SQL transaction can
   commit.** `AgentTaskService.createTask`,
   `DocumentIngestionService.submitIngestion`,
   `DocumentIngestionTaskService.retryTask`, and
   `DocumentReindexService.submitReindex` are transactional and call a Rabbit
   publisher in the method body. A message can escape while later database
   work or commit fails, or durable state can be marked failed although broker
   acceptance is uncertain. The message contains only numeric IDs and has no
   committed revision/attempt/fence with which a consumer could reject this
   ghost delivery. These paths and their tests are DROP.

2. **MQ messages cannot identify the execution they authorize.** All three
   message types are mutable numeric-ID holders. Consumers pass those IDs to a
   service with no expected revision, claim token, attempt, idempotency key, or
   source-operation identity. At-least-once delivery therefore relies entirely
   on the already-insufficient Java status checks described in review 02.

3. **Outbox completion is not fenced after takeover.**
   `TaskOutboxDispatcherScheduler.java:54-79` claims, sends, then calls
   `markSent(outboxId)` or `markFailed(outboxId, error)`. Neither call carries
   the `lockedBy` value or a claim generation. Once a 60-second stale lock is
   reclaimed, the previous dispatcher can finalize the new owner's row.

4. **Timeout recovery is a status write, not attempt recovery.**
   `TaskTimeoutScheduler.java:30-45` scans `RUNNING` rows by host-local time and
   asks `TaskService` to mark each ID timed out. It does not observe and compare
   a lease token, record the exact abandoned attempt, or prevent an old worker
   from becoming valid again after a later retry returns the row to `RUNNING`.

## P1 findings

1. `TaskRetryScheduler.java:31-69` scans and handles a batch under one outer
   transaction, calls its own `@Transactional` method by self-invocation, and
   catches per-item exceptions. A database error may mark the shared
   transaction rollback-only, causing apparently successful later items to
   roll back together. There is no committed per-item checkpoint.
2. All schedulers call `LocalDateTime.now()` directly. Durable due/timeout
   behavior depends on host timezone and cannot be deterministically tested
   with an injected UTC clock.
3. Fixed five-second polling, a fixed 20-row batch, and a fixed 60-second
   takeover threshold are embedded in code. There is no bounded shutdown or
   explicit quiesce ownership in this layer.
4. `TaskOutboxDispatcherScheduler` catches arbitrary exceptions, passes the
   raw exception into durable failure handling, and logs it. The primary test
   requires persistence of `rabbit down`, which is not a safe failure-code
   contract.
5. `TaskTimeoutScheduler` and `TrashCleanupScheduler` have no direct tests in
   the frozen tree. Existing scheduler tests are Mockito call-order checks;
   they do not prove broker acknowledgment, process interruption, takeover,
   stale-owner rejection, clock movement, transaction rollback, or shutdown.
6. `BatchIngestionIntegrationTest` accepts any of three nonterminal statuses
   and mocks publication. Agent tests publish directly after mock persistence.
   Neither establishes one durable operation or exactly one terminal outcome.
7. The helpful comment that a broker payload should not become a second data
   truth is insufficient: Rabbit delivery state and SQL task/outbox state are
   still two failure domains requiring reconciliation.

## Retained Go ownership

No Java MQ or scheduler symbol is retained. Current Go ownership is:

- `v2/internal/store/sqlite/jobs.go`: one transactionally claimed local job
  row, opaque lease token, cancellation precedence, terminal immutability, and
  expired/startup recovery;
- `v2/internal/store/sqlite/documents.go`: document admission and ingestion
  work created atomically, with leased finalization and revision activation;
- application lifecycle code: context cancellation and ordered shutdown of
  local workers before storage close.

This is a local wake-up loop over durable SQLite state, not a translation of
Rabbit queues, outbox tables, Spring `@Scheduled`, or the Java status enum.

## Verification

`build-review-03.ps1` derives the ledger from the global frozen manifest and
fails unless the exact counts are 17 primary plus eight cross-reference rows
with unique paths. `freeze-manifest.ps1` fails if any Java/resource input
differs from the baseline. No production, Java, test, data, backup, retrieval,
or release file was changed.
