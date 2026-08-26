# Review 02: entities, repositories, storage, and Flyway V1-V33

Status: complete at `0df22ddaf02c64bf73a7df12cd5fea6b52632c73`.

This is a deletion-oriented review. All Java/JPA/storage implementations,
their tests, and the complete MySQL/Flyway history are `DROP`. Nothing in this
report authorizes importing a Java row, identifier, status, encrypted value,
prompt, source body, error message, path, or migration into a fresh Go Vault.

## Frozen review set

`review-02-files.csv` is the authoritative file ledger. It binds every row to
the baseline, path, SHA-256, byte count, line count, full `1-N` range, scope
class, and disposition. Counts intentionally separate the exact-set owned by
this review from tests in other packages that merely exercise data types:

| Scope | Kind | Files | Lines | Reviewed range |
| --- | --- | ---: | ---: | --- |
| primary | `entity`, `repository`, `storage` production Java | 69 | 3,855 | every file, line 1 through EOF |
| primary | Flyway `V1` through `V33` | 33 | 749 | every file, line 1 through EOF |
| primary | tests physically under `repository` or `storage` | 8 | 978 | every file, line 1 through EOF |
| cross-reference | tests in other packages importing an entity/repository/storage type | 74 | 10,239 | every file, line 1 through EOF |
| **Reviewed evidence total** |  | **184** | **15,821** | **complete** |

The primary exact-set is 110 files / 5,582 lines. Cross-reference rows are not
counted again in the platform/data primary total. They are historical evidence
only and remain owned by their physical package for the final repository-wide
accounting.

The SHA-256 of `review-02-files.csv` is
`f8c7c62efa7cd5fe9ae9b86657fdf12451f6a3008817c5416acf275d502f8e02`.

## Disposition

- All 33 JPA entities: `DROP`.
- All 33 Spring Data repositories: `DROP`.
- All three Java storage services: `DROP`.
- All 33 MySQL migrations, including seed rows: `DROP`.
- All eight primary tests and 74 cross-reference tests: `DROP` as tests and as
  contracts. Mock expectations are not accepted product requirements.

There is no implementation exception. A semantic marker in the CSV means only
that the named behavior is already independently owned and tested by the Go
fresh-Vault implementation; it never means reuse or translate the Java type.

## P0 findings

1. **Task finalization is vulnerable to a stale execution owner.**
   `TaskRepository.java:26-142` claims and finalizes by task ID plus status.
   It has no lease owner, lease token, writer incarnation, attempt stamp, or
   fencing comparison. After a timeout/takeover, an old worker can satisfy a
   later `RUNNING` predicate and write a success, retry, timeout, or failure.
   `TaskRepositoryFinalizationTest` and `TaskAtomicClaimConcurrencyTest` prove
   only status CAS and simultaneous first claim, not stale-owner rejection.

2. **Outbox takeover does not fence the previous dispatcher.**
   `TaskOutboxRepository.java:38-99` records `lockedBy` while claiming, but
   `markSent` and `markFailed` compare only ID and `PROCESSING`. A reclaimed
   row can therefore be finalized by the old dispatcher. The repository test
   checks claim contention but never proves an old owner is rejected after
   takeover. Rabbit/outbox delivery is not part of local CORE and is DROP.

3. **Attempt allocation is a read-then-increment race.**
   `TaskAttemptRepository.java:14-19` exposes `max(attemptNo)` for application
   allocation. A unique constraint detects a collision after the fact; it does
   not atomically allocate an attempt or bind it to a claim. Sequential mock
   tests do not prove concurrent allocation or abandoned-attempt recovery.

4. **Permanent cleanup performs external I/O inside a database transaction
   and deliberately continues after failures.**
   `StorageCleanupService.java:45-83` calls the vector store under
   `@Transactional`, catches its error, then deletes database embeddings,
   chunks, memberships, and source text. It also catches per-cache errors in
   `:86-109`. This can commit partial purge state and residue, and warnings
   concatenate raw exception messages. Its test covers only the happy call
   sequence. The implementation is DROP.

5. **Durable tables normalize storage of unrestricted sensitive content.**
   The entity/migration chain persists prompt and rendered prompt, result
   bodies, source text in two places, chunk content/snippets, free-form event
   metadata, input/output JSON, provider errors, and staging paths. Examples
   include `V1`, `V3`, `V8`, `V11`, `V12`, `V15`, `V16`, `V19`, `V20`, `V24`,
   `V28`, and `V29`. Tests positively assert several of these raw values. None
   is migration input for a fresh Vault.

6. **Migrations seed product behavior and LATER features into durable state.**
   `V7` inserts a default prompt containing user input. `V24-V25` introduce
   Agent tasks/steps. `V29-V32` introduce evaluation and vector audit/reindex
   machinery. `V33` introduces Agent profiles, system instructions, and
   memory. Those seeds and features are `DROP`, not bootstrap requirements.

## P1 findings

1. All entities use mutable numeric JPA identity and `LocalDateTime`; none has
   an aggregate revision (`@Version`) or a complete immutable execution stamp.
2. Early migrations add many relationships without foreign keys, checks,
   bounded JSON/content, non-negative counters, or closed status domains.
   Later constraints do not retroactively make the model a safe import source.
3. `V26` permits multiple `is_default` model providers; application code
   clears and resets defaults instead of enforcing one active row in storage.
4. `V31` attempts a scope uniqueness constraint on nullable
   `(collection_id, document_id, generation)`. Under MySQL NULL uniqueness it
   neither enforces an XOR scope nor prevents all duplicate nullable scopes.
5. `DocumentEntity` and `DocumentIngestionTaskEntity` duplicate source text.
   Several other projections duplicate lifecycle/generation/vector truth
   across SQL and an external vector store without an atomic reconciliation
   protocol.
6. `StorageSummaryService.java:29-57` combines SQL character-length estimates
   and counts while excluding external vector bytes; it is not an authoritative
   capacity or cleanup report. `CacheManagementService` also reports retrieval
   cache clearing although that cache is absent.
7. Collection creation uses existence-check-then-save and broad repository
   reads appear throughout. Database uniqueness may reject some races, but the
   service/tests do not establish stable conflict mapping, bounded scans, or
   crash convergence.
8. The cross-reference tests are predominantly Mockito interaction tests.
   They do not prove process interruption, transaction boundaries around
   external calls, disk faults, exact residue recovery, or bounded persisted
   content. Manual Qdrant tests additionally require network state and are not
   offline evidence.

## Flyway disposition by range

| Versions | Historical shape | Decision |
| --- | --- | --- |
| `V1-V11` | tasks, events, retries, cancellation, prompt templates, result chunks | DROP schema, rows, statuses, and seeds |
| `V12-V23` | documents, chunks, embeddings/cache, outbox/attempts, ingestion, collections | DROP MySQL/JPA shape; retain only separately proven Go invariants below |
| `V24-V26` | Agent tasks/steps and model-provider credentials/configuration | DROP |
| `V27-V28` | trash/purge, upload batches, notifications, staging paths | DROP schema; lifecycle value exists only through accepted Go behavior |
| `V29-V32` | RAG evaluation, chunk metadata, vector generations/audit/reindex | DROP |
| `V33` | Agent profiles and memory foundation | DROP |

No Flyway version is replayed, translated, or used to create a Go Vault. The
Go schema starts from its own version 1 and accepts no Java/MySQL data path.

## Requirements retained only because Go already proves them

The narrow retained requirements are evidence-linked to current Go code:

- `v2/internal/store/sqlite/jobs.go` and `jobs_test.go`: a claim issues an
  opaque lease token; all worker commits compare that token and unexpired
  running state; cancellation and terminal states win; startup and expired
  work converge without accepting a stale worker.
- `v2/internal/store/sqlite/documents.go`, `documents_test.go`, and migrations
  `001-006`: upload admission atomically creates the document, inactive
  revision, ingestion job, and idempotency relation; ingestion atomically
  inserts chunks, activates exactly one revision, and finalizes its leased job.
- `v2/internal/store/sqlite/lifecycle.go`, `lifecycle_test.go`, and migration
  `005_document_lifecycle.sql`: trash/restore/purge transitions, tombstones,
  blob-GC candidates, and retryable residue are durable and constrained.
- `v2/internal/blob/store.go`, `maintenance.go`, and their tests: content
  address, size limit, publish durability, identity re-checks, pins, and
  deletion serialization own object storage behavior.

The useful idea that external provider I/O must not be enclosed in a long SQL
transaction is also preserved as a Go design rule. The Java Rabbit outbox and
the Java purge implementation are not preserved to express it.

## Verification

`build-review-02.ps1` derives the ledger from the frozen global manifest and
fails unless the exact counts are 110 primary plus 74 cross-reference rows
with unique paths. `freeze-manifest.ps1` separately fails closed if any frozen
Java/resource input differs from the baseline. The review changes no product,
Java, test, migration, backup, retrieval, or release file.
