# Legacy Java core review evidence

## Decision

This evidence freezes a full-file semantic review of the selected legacy Java
core/data surface at commit
`0df22ddaf02c64bf73a7df12cd5fea6b52632c73`.

The result is intentionally strict:

- production implementation `KEEP`: **0 files / 0 lines**;
- production implementation `KEEP_WITH_HARDENING`: **0 files / 0 lines**;
- reviewed implementation `DROP`: **330 files / 26,689 physical lines**;
- Java source: **238 files / 16,900 lines**;
- Java tests: **92 files / 9,789 lines**.

`DROP` means that no Java class in this scope is a porting unit. A small set of
product invariants remains useful, but those invariants must be expressed as Go
contracts and independently qualified against the Go runtime. They do not make
the Java implementation reusable.

## Reproducible binding

The normative per-file record is [files.csv](files.csv). It binds every reviewed
file to:

- the full source commit;
- an ordinal sequence and exact repository-relative path;
- the Git blob object ID;
- the raw blob SHA-256 and byte count;
- the physical line count and the reviewed `1-N` range;
- `FULL_FILE` coverage and `DROP` implementation disposition;
- primary feature, risk, drop-reason, semantic-requirement, evidence, and
  cross-file-flow IDs.

The CSV is canonical LF-terminated UTF-8 without BOM, quotes, commas inside
fields, or blank fields.

| Binding | SHA-256 |
|---|---|
| `files.csv` raw bytes | `0c1d9f4bf013783070077555e26f613ce2082f1f233f3056e0fec015b7b091bf` |
| ordered `path<TAB>blob<TAB>raw_sha256<TAB>lines<LF>` projection | `b5d0d0a6aaca27fb2a750c2603c0269d9eedd51d38fba8bc2c073ce595c35c04` |

The verifier derives the expected path set and blob IDs from the frozen commit;
it does not trust the CSV for scope discovery.

## Exact scope

Only these top-level Java partitions are owned by this review. Configuration,
security, common state, entities, repositories, storage, messaging, schedulers,
Flyway, profiles, and the other feature packages are outside this evidence set.

| Partition | Files | Lines | Coverage |
|---|---:|---:|---|
| main root application | 1 | 25 | FULL_FILE |
| main controller | 23 | 1,446 | FULL_FILE |
| main service | 47 | 9,902 | FULL_FILE |
| main document | 20 | 1,805 | FULL_FILE |
| main dto | 102 | 3,241 | FULL_FILE |
| main enums | 44 | 428 | FULL_FILE |
| main prompt | 1 | 53 | FULL_FILE |
| test root | 1 | 13 | FULL_FILE |
| test controller | 16 | 1,847 | FULL_FILE |
| test service | 47 | 5,945 | FULL_FILE |
| test document | 11 | 561 | FULL_FILE |
| test lifecycle | 9 | 877 | FULL_FILE |
| test documentation | 5 | 228 | FULL_FILE |
| test staticresource | 2 | 233 | FULL_FILE |
| test prompt | 1 | 85 | FULL_FILE |
| **Total** | **330** | **26,689** | **FULL_FILE** |

The source set is selected mechanically as follows:

- main root `AiTaskOrchestratorApplication.java`;
- main top-level packages `controller`, `service`, `document`, `dto`, `enums`,
  and `prompt`;
- test root `AiTaskOrchestratorApplicationTests.java`;
- test top-level packages `controller`, `service`, `document`, `lifecycle`,
  `documentation`, `staticresource`, and `prompt`.

## Package disposition

| Surface | Disposition | Reason |
|---|---|---|
| root application | DROP | Spring runtime entry point is retired and carries no portable domain boundary. |
| controllers | DROP | Thin unversioned pass-through APIs with unbounded lists and no uniform idempotency or revision contract. |
| services | DROP or rewrite from requirements | State transitions and external side effects do not share one durable fenced protocol. |
| document | DROP | Unsafe text/PDF/resource boundaries and duplicate chunking implementations. |
| DTOs | DROP | Mutable unbounded transport bags expose raw errors and accreted incompatible response shapes. |
| enums | DROP | Speculative and contradictory states do not constitute an enforced state machine. |
| prompt | DROP | Regex substitution and prompt assembly lack total budgets and safe untrusted-context delimiting. |
| tests | DROP as implementation | Mostly mocked, transactional, happy-path, or static-text assertions; none closes a product flow. |

## P0 cross-file findings

### P0-RAG-SCOPE-WIDEN — `FLOW-RAG-SCOPE`

- `AppRetrievalService.java:113-117` lets a routing-derived collection override
  the explicit request collection.
- `DocumentLifecycleFilterService.java:153-155` treats both `null` and an empty
  allowed-document set as allow-all.
- The reviewed tests do not cover a routing/request mismatch or prove that an
  explicitly empty scope remains empty through the whole Ask path.

Required replacement invariant: `SEMANTIC-SCOPE-NO-WIDEN` plus
`SEMANTIC-LIFECYCLE-FILTER`.

### P0-RAG-UNTRUSTED-OUTPUT — `FLOW-RAG-ANSWER`

- `RagAnswerService.java:356-381` calculates verification and a post-verification
  refusal only as diagnostics.
- `RagAnswerService.java:388-397` still returns the raw model content.
- `RagAnswerService.java:597-608` preserves a citation whose chunk is absent
  from the final context instead of rejecting it.

Required replacement invariant: `SEMANTIC-CITATION-FINAL-CONTEXT`; invalid
verification must alter or withhold the answer, not merely decorate it.

### P0-INGESTION-DOUBLE-EXECUTION — `FLOW-INGESTION-DUAL-WRITE`

- `DocumentIngestionTaskHandler.java:57-84` reads `PENDING` and later writes
  `PROCESSING`; `:302-315` performs no atomic claim, lease, or fencing check.
- message publication occurs inside database transactions at
  `DocumentIngestionService.java:136`,
  `DocumentIngestionTaskService.java:101`,
  `DocumentReindexService.java:95`, and `AgentTaskService.java:91`.
- A consumer can race uncommitted state, while publish success followed by
  transaction failure creates an uncertain duplicated outcome.

Required replacement invariant: `SEMANTIC-JOB-CLAIM-CAS`,
`SEMANTIC-JOB-FENCE`, and `SEMANTIC-ATOMIC-ENQUEUE`.

### P0-REINDEX-ACTIVE-LOSS — `FLOW-REINDEX-ACTIVE`

- `DocumentIngestionTaskService.java:82-103` retries every failed task by calling
  `clearChunksForRetry`, including failed reindex tasks.
- `DocumentIngestionTaskHandler.java:203-208` activates the new generation
  before the vector reindex completion callback. Failure after activation can
  delete new chunks while old chunks are already superseded.
- Tests cover ordinary success and mock failure shapes, not a crash/fault at
  every activation boundary.

Required replacement invariant: `SEMANTIC-ACTIVE-GENERATION` with a single
durable activation CAS.

### P0-PURGE-OUTCOME-UNCERTAIN — `FLOW-LIFECYCLE-PURGE`

`DocumentTrashService.java:88`, `:120`, and `:161` invoke vector/blob side
effects inside database transactions. There is no immutable purge plan,
operation receipt, fencing token, residual verification, or explicit uncertain
outcome. A rollback cannot undo the external side effect.

Required replacement invariant: `SEMANTIC-PURGE-VERIFICATION`.

### P0-PROVIDER-EGRESS — `FLOW-PROVIDER-EGRESS`

`ModelProviderTestService.java:73-74` and `:105-108` construct clients from a
stored arbitrary base URL and send provider requests, including authenticated
requests, without a scheme/host/address/redirect/DNS-pinning policy or a bounded
response contract. This is an SSRF and secret-exfiltration boundary.

Required replacement invariant: `SEMANTIC-PROVIDER-EGRESS` with loopback-only
policy and post-authorization address pinning.

### P0-BATCH-ATOMICITY — `FLOW-BATCH`

`UploadBatchService.java:80-125` writes staging state and invokes the batch
runner before its transaction commits. Additional callbacks invoke that runner
from transactions at `:270`, `:279`, `:294`, and `:343`. Rollback, cancellation,
message delivery, and staging cleanup therefore do not form one recoverable
state machine.

Required replacement invariant: `SEMANTIC-ATOMIC-ENQUEUE`.

### P0-TASK-OUTCOME-UNCERTAIN — `FLOW-TASK-OUTBOX`

- LLM/provider invocations have no durable invocation receipt; a completed
  provider call with a lost response is retried as if known not to have run.
- `TaskAttemptService.java:35-47` calculates `max(attemptNo)+1` without a
  serialized allocation boundary.
- outbox finalization does not bind a dispatcher owner/fencing token; publish
  success followed by `markSent` failure can duplicate delivery.
- output chunks split Java UTF-16 substrings and can be duplicated across retry.

The isolated task-claim test is useful evidence for a primitive, not evidence
that this workflow closes.

## P1 structural findings

- `DocumentChunker.java:50-103` uses UTF-16 offsets and substring boundaries;
  surrogate pairs can be split, overlap can cross section boundaries, and
  heading metadata can describe different bytes.
- `StructuredChunkingService` duplicates chunking and substitutes crude
  character/token/language heuristics for a bounded canonical algorithm.
- `TxtTextExtractor.java:25` silently replaces malformed UTF-8.
- `PdfTextExtractor.java:30-46` parses untrusted PDF in-process with no page,
  decoded-size, memory, or execution budget and returns raw exception text.
- `FileHashService.java:17-21` trims and normalizes text before hashing, so the
  digest is not source-byte identity.
- `DocumentIngestionEventRecorder` persists filename/error/metadata in a
  separate transaction and silently degrades serialization failures; state and
  event cannot prove each other.
- many services and controllers use unpaginated `findAll`/list operations.
- embedding/vector replacement holds database transactions across external
  work and deletes old state before durable activation.
- DTOs expose mutable nullable shapes, raw diagnostic content, provider URLs,
  chunks, hashes, and error messages without one versioned compatibility model.
- enums include contradictory spellings and speculative Agent/Memory/Evaluation
  state surfaces without transition enforcement.
- RAG prompt construction has no whole-prompt budget or robust delimiter for
  untrusted document and memory content.
- evaluation imports use naive CSV splitting and weak schema bounds; retrieval
  metrics do not deduplicate retrieved IDs, so duplicate hits can inflate recall.

## Semantic requirements retained as requirements only

The CSV associates every file with one primary requirement ID. These IDs are
not implementation salvage decisions.

| ID | Go-side requirement |
|---|---|
| `SEMANTIC-SCOPE-NO-WIDEN` | An explicit collection/document set, including an empty set, can never widen. |
| `SEMANTIC-LIFECYCLE-FILTER` | Trash and inactive revisions/generations are never visible to retrieval. |
| `SEMANTIC-JOB-CLAIM-CAS` | Exactly one current execution owns a durable claim and terminal CAS. |
| `SEMANTIC-JOB-FENCE` | Every durable write is bound to the current lease/fencing identity. |
| `SEMANTIC-ATOMIC-ENQUEUE` | State and executable work are committed together or recovered explicitly. |
| `SEMANTIC-CITATION-FINAL-CONTEXT` | Returned citations are an exact subset of final model context; invalid verification changes output. |
| `SEMANTIC-PROVIDER-EGRESS` | Secrets are not returned and authenticated egress is local-policy authorized and address-pinned. |
| `SEMANTIC-RUNE-SAFE-CHUNKING` | Chunk boundaries and identities are deterministic and Unicode-rune safe. |
| `SEMANTIC-STRICT-UTF8` | Invalid source text fails closed rather than being silently replaced. |
| `SEMANTIC-SOURCE-BYTE-IDENTITY` | Source identity hashes raw bounded bytes, not trimmed display text. |
| `SEMANTIC-PDF-ISOLATION` | Untrusted PDF parsing is a bounded isolated helper capability. |
| `SEMANTIC-ACTIVE-GENERATION` | Old content remains active until one durable new-generation activation CAS. |
| `SEMANTIC-PURGE-VERIFICATION` | Purge uses an immutable plan, receipt, fencing, and residual-zero verification. |
| `SEMANTIC-BOUNDED-PROMPT` | Prompt components and total assembled prompt are bounded and safely delimited. |
| `SEMANTIC-ERROR-REDACTION` | Durable and user-facing failures contain stable safe codes, not raw secrets/content/paths. |
| `SEMANTIC-FILE-BOUNDS` | File type, source bytes, extracted bytes, and resource work are bounded before admission. |
| `SEMANTIC-MEMORY-LATER` | Memory/Agent behavior remains outside CORE and requires a new user-confirmed design. |
| `SEMANTIC-NONE` | No independent product requirement is salvaged from that file. |

## Test evidence assessment

The 92 test files were also read in full. Their implementation disposition is
`DROP`. Structural annotations overlap, but the reviewed set contains 32
`@SpringBootTest` files, 13 `@WebMvcTest` files, 21 Mockito-extension files, 17
transactional tests, and 16 files in the documentation/staticresource/lifecycle
contract partitions. There
is one real concurrency harness and no process-kill/restart/power-loss or
cross-boundary fault matrix in this scope.

Only ten files are marked `independent_evidence=LIMITED`:

- `TaskAtomicClaimConcurrencyTest` — one-winner pending-task claim;
- `TaskExecutionFinalizationIntegrationTest` and
  `TaskServiceFinalizationTest` — basic terminal CAS behavior;
- `DocumentFileValidatorTest` — elementary type/size bounds;
- `DocumentTextExtractorTest` — elementary UTF-8/Markdown and simple English PDF;
- `DocumentEmbeddingServiceCacheIntegrationTest` — cache reuse with a mocked provider;
- `DocumentLifecycleFilterServiceTest` and `RetrievalFilterRegressionTest` —
  basic lifecycle ID filtering, not the whole scope flow;
- `ModelProviderConfigServiceTest` — response masking, not at-rest or egress safety;
- `AgentMemoryIsolationTest` — basic database scope behavior for a non-CORE feature.

`LIMITED` never changes `implementation=DROP`. Every other test is marked
`independent_evidence=NO` because it is mocked, happy-path, transactional,
static-text, or does not exercise the claimed boundary.

No legacy Java test suite was rerun to manufacture new qualification evidence.
The review decision is based on full source/test reading at the bound commit;
the verifier checks the integrity and completeness of that reading manifest.

## Verification

From the repository root with PowerShell 7:

```powershell
pwsh -NoProfile -File docs/rewrite/evidence/legacy-core-review/verify.ps1
pwsh -NoProfile -File docs/rewrite/evidence/legacy-core-review/verify.ps1 -SelfTest
```

The normal verifier fails closed on commit ancestry/binding, manifest wire hash,
UTF-8/canonical line form, exact path set/order, duplicates, blob IDs, raw
SHA-256, byte counts, physical line counts, `1-N` ranges, FULL_FILE coverage,
allowed IDs, and any disposition other than `DROP`.

`-SelfTest` additionally proves rejection of a modified wire hash, wrong source
commit, wrong line range, missing exact-set member, duplicate path, wrong raw
source SHA-256, and any `KEEP` disposition.

Expected success summaries:

```json
{"schema":"mindweaver.legacy-core-review.v1","status":"PASS","files":330,"lines":26689,"implementation_keep":0,"implementation_harden":0,"implementation_drop":330}
{"schema":"mindweaver.legacy-core-review.self-test.v1","status":"PASS","checks":7}
```

## Non-claims

- This evidence does not qualify, build, run, or publish the legacy Java app.
- It does not authorize copying any Java implementation into Go.
- It does not review the explicitly excluded Java partitions.
- It does not claim that a matching behavior is already complete in Go; each
  retained semantic requirement still needs Go-owned tests and release evidence.
- It does not alter Java, Go production code, the existing inventory, or any
  acceptance ledger.
