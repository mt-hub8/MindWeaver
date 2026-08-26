# Review 04: platform/data deletion summary

Status: complete at `0df22ddaf02c64bf73a7df12cd5fea6b52632c73`.

The platform/data Java review is complete. Whole-source `KEEP` count is zero.
Every reviewed Java implementation, Java test, MySQL migration, and application
profile is `DROP`. The fresh-Vault Go product imports none of their code, rows,
schema history, credentials, ciphertext, status names, prompts, content, or
runtime topology.

## Exact-set and coverage

`review-04-files.csv` is the final deduplicated ledger. Every row has the exact
baseline, repository path, SHA-256, byte count, line count, complete `1-N`
range, primary/cross-reference class, first review source, and disposition.

The primary exact-set follows the ownership rule literally:

- production Java under `config`, `security`, `common`, `state`, `entity`,
  `repository`, `storage`, `mq`, and `scheduler`;
- resources under `src/main/resources/db` plus files directly in
  `src/main/resources` (the four application profiles); and
- tests physically under `common`, `repository`, `scheduler`, `security`,
  `state`, and `storage`.

`src/main/resources/static` is not in this platform/data primary exact-set.
Tests in other packages are cross-reference evidence even when they start a
Spring context, read a profile/Flyway resource, or import a target type.

| Scope | Kind | Files | Lines | Disposition |
| --- | --- | ---: | ---: | --- |
| primary | nine production Java packages | 101 | 5,395 | DROP |
| primary | Flyway V1-V33 | 33 | 749 | DROP |
| primary | root application profiles | 4 | 182 | DROP |
| primary | six direct test packages | 13 | 1,441 | DROP |
| **primary total** |  | **151** | **7,767** | **DROP** |
| cross-reference | all other associated Java tests | 121 | 14,974 | DROP |
| **frozen evidence universe** |  | **272** | **22,741** | **DROP** |

The SHA-256 of the global `frozen-file-manifest.csv` is
`429e67f008f9af12719bfa0f390c7a1c85ddbbc34396d409cae0f0306d31e61f`.
The SHA-256 of final `review-04-files.csv` is
`fcf89eeae9d20672c5e9e19a981c4b88bb022954ad0e4f186ef2ad732abb1068`.

The per-partition row totals overlap only in cross-reference tests. The final
ledger assigns every path once: 70 files first covered by review 01, 161 by
review 02 after deduplication, 15 production files by review 03 after
deduplication, and 26 context/profile-only test files by this final supplement.
Their union is exactly 272/272 frozen paths.

## Final KEEP / SIMPLIFY / DROP decision

### KEEP: requirements only, with existing Go proof

No Java file is kept. Only these behavior requirements survive, because the Go
fresh-Vault implementation already owns and tests them:

1. Versioned validated local configuration, context-aware I/O, injected time
   in durable logic, and stable bounded transport problems.
2. A SQLite job is the single durable local work truth: atomic claim, opaque
   lease token, cancellation precedence, stale-worker rejection, terminal
   immutability, and startup/expired recovery.
3. Upload admission atomically creates document identity, inactive revision,
   ingestion job, idempotency relation, and source-blob reference. Successful
   ingestion activates one revision and finalizes the same leased job in one
   transaction.
4. Trash, restore, purge-pending, tombstone, and blob-GC residue converge
   durably; content-addressed objects are size bounded, identity checked, and
   serialized against deletion.
5. Provider/filesystem/process I/O is never enclosed by a long SQL transaction.
   External outcomes use explicit reconciliation rather than pretending to be
   atomic with SQLite.

The proof owners are `v2/platform/config`, `v2/platform/apperror`,
`v2/internal/transport/problem.go`, `v2/internal/store/sqlite/jobs.go`,
`documents.go`, `lifecycle.go`, schema `001-006`, `v2/internal/blob`, and their
accepted tests. Those Go owners are authoritative; similar Java behavior is
not corroborating production code.

### SIMPLIFY: replacement shape, not translation

- Spring profiles and mutable property binders become the smaller versioned
  Go configuration surface.
- JPA entities/repositories and 33 historical MySQL migrations become the
  fresh Go SQLite schema only; no migration bridge exists.
- Rabbit producers/consumers/outbox and four Spring schedulers become one local
  context-aware worker over SQLite jobs.
- Java cleanup ordering becomes durable purge intent plus retryable blob-GC
  residue; external deletion never shares the database transaction.
- Broad mutable projections become narrow store/application structs required
  by current CORE operations.

### DROP: no requirement extraction

- Java/MySQL data migration, Flyway replay/translation, third PE, and a
  `migrate` command;
- Spring Boot, JPA/Hibernate, MySQL, RabbitMQ, Python worker, Qdrant topology,
  Java secret/ciphertext format, and committed profile values;
- Agent tasks/profiles/tools, memory, vector generation/reindex/audit,
  evaluation/benchmarking, batch notification orchestration, prompt templates,
  and mock-provider seeding;
- Java status/error enums, raw error/event content, source-text duplication,
  staging paths, prompt/result persistence, and static-copy/documentation tests;
  and
- all Java tests as Go acceptance criteria. Mock interaction and Spring
  context success do not prove crash consistency, fail-closed recovery, or
  bounded offline operation.

## Consolidated P0

These are prohibitions and deletion reasons. They are not requests to repair
the old implementation:

1. Never ship or import committed profile credentials, Java AES ciphertext, or
   the property/environment master-key convention.
2. Never expose the Java exception adapter: it returns/logs raw error, provider,
   path, and untrusted request-ID text.
3. Never reuse the Java task/outbox protocol: claim/finalization lacks an
   execution fence, stale owners can finalize after takeover, and attempt
   allocation is a read-then-increment race.
4. Never reuse Java purge: it performs vector I/O under `@Transactional`,
   swallows failures, then continues destructive database cleanup.
5. Never replay V1-V33: they persist unrestricted prompt/source/result/error/
   path content, contain weak or ineffective constraints, and seed prompts,
   Agent profiles, system instructions, and memory.
6. Never reuse direct Rabbit publication from transactional Agent/document/
   retry/reindex methods. A message can escape before commit and carries no
   revision, attempt, lease token, or fence.
7. Never treat timeout/status scanning as recovery. It does not preserve and
   abandon the observed attempt or reject the prior worker after a later retry.

No P0 requires porting old code. The safe resolution is the already-decided
DROP plus the independent Go invariants above.

## Consolidated P1

1. Mutable JPA entities have no aggregate revision; durable time is host-local
   `LocalDateTime`; configuration is unversioned and incompletely bounded.
2. Early migrations lack complete FK/check/status/size/counter constraints;
   `V26` does not enforce one default provider and `V31` nullable scope
   uniqueness does not enforce its intended identity.
3. Storage estimates are not byte-authoritative and omit external vector use.
   Cache clearing reports behavior for a cache that is not implemented.
4. Scheduler constants are embedded, clock injection is absent, retry batches
   share transaction fate, and timeout/trash schedulers lack direct tests.
5. Context/profile-only tests freeze mock providers, Java+Python manuals,
   LATER Agent/memory/vector/evaluation behavior, UI copy, and repository paths.
   Their successful Spring context does not prove a supported deployment.
6. Many tests positively persist raw source, prompt, result, or provider error
   text. These fixtures are disclosure risks, not compatibility requirements.

## Commit and integration order

The evidence stack is docs-only and must be applied in order because later
ledgers consume the frozen manifest introduced by the first commit:

1. `15461f0873646ec7b7ef4ec0bf2a8469412aa206` — config/security/common/state,
   profiles, and global frozen manifest.
2. `f8508f52cd011442c5d61954eae66d35676fd9af` — entity/repository/storage and
   Flyway V1-V33.
3. `f0f835c292a5fc9a17ad059ca61e3eb475aaac84` — MQ and schedulers.
4. this summary commit — deduplicated 272-file closure.

No commit changes Java, Go production, backup, migration, retrieval, release,
or vendor content.

## Verification

The evidence generators fail closed when the baseline is not an ancestor,
when any reviewed Java/resource input differs from the baseline, when a path
or range is missing/duplicated, or when exact counts change. Final validation
must prove:

- primary = 151 files / 7,767 lines;
- cross-reference = 121 files / 14,974 lines;
- union = global manifest = 272 files / 22,741 lines;
- every SHA-256/byte/line/`1-N` value equals the global frozen row; and
- every implementation disposition is `DROP`.
