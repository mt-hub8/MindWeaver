# Evaluation, Batch, and Knowledge Health source review

## Frozen baseline and scope

- Baseline: `0df22ddaf02c64bf73a7df12cd5fea6b52632c73`
- Machine-readable evidence: [`evaluation-batch-kbhealth.csv`](./evaluation-batch-kbhealth.csv)
- Verification: `pwsh -NoProfile -File docs/rewrite/evidence/legacy-product-review/verify.ps1 -Manifest docs/rewrite/evidence/legacy-product-review/evaluation-batch-kbhealth.csv`
- Every row covers the complete Git blob (`1..line_end`) and records its SHA-256.

The direct set is exact: all 53 production files under `evaluation`, `batch`,
and `kbhealth`; all 22 files in the corresponding test packages; and both
files under `src/test/resources/evaluation`. Named controllers, DTOs, entities,
enums, repositories, services, schemas, and integration tests freeze the
product adapters that make those packages reachable.

| Surface | Files | Lines | Decision |
| --- | ---: | ---: | --- |
| Legacy direct production packages | 53 | 4,419 | `DROP` |
| Legacy product adapters | 61 | 5,463 | `DROP` |
| Legacy MySQL schema | 3 | 233 | `DROP` |
| Legacy static UI dependency evidence | 8 | 976 | `DROP` |
| Legacy direct tests | 22 | 3,078 | `DROP` |
| Legacy evaluation test resources | 2 | 57 | `DROP` |
| Legacy adapter tests | 13 | 1,568 | `DROP` |
| Current Go negative-surface contract/test | 2 | 1,678 | `HARDEN` |
| **Partition-local total** | **164** | **17,472** | frozen in CSV |

The eight static rows are dependency evidence only. They are not an allocation
of the static UI and must not be added to other partition-local static counts.
The final static master review owns each of the 47 static files exactly once,
with the required de-duplicated total of 10,223 lines.

There are zero Java implementation exceptions. Evaluation, Batch, and
Knowledge Health remain future zero-to-one product decisions, not code to port.

## Findings

### P1: an evaluation run is synchronous provider work inside one transaction

`RagEvaluationRunService.java:100-197` creates a `RUNNING` row, executes every
retrieval and optional generation case, stores results, and completes the run
inside one transaction. The loop checks only the already-loaded object's status
at lines `147-150`; the cancel endpoint at `217-223` cannot make that object
observe a concurrent update. A process kill can roll back the run and its case
evidence after external provider effects have happened, while a slow provider
holds database resources. There is no durable claim, request fingerprint,
provider/config snapshot, cancellation checkpoint, cost budget, or
outcome-uncertain state.

The same service writes raw `exception.getMessage()` into case results at
lines `161-172`. Provider messages, endpoints, queries, or source fragments can
therefore become durable report content without a content-free error contract.

### P1: run comparison violates its stated dataset invariant

The class states at `RagEvaluationRunService.java:61-62` that comparison is
meaningful only for the same Dataset/Case set. `compareRuns` at `225-286` loads
two arbitrary run IDs and compares their summaries without checking dataset ID,
case identity, corpus digest, retrieval snapshot, model/provider configuration,
or pipeline version. It can recommend adopting a candidate strategy from
incomparable inputs.

### P1: missing or corrupt metrics are reported as a healthy knowledge base

`KnowledgeBaseDiagnosisService.java:97-100` and `:122-125` silently skip every
missing or unavailable metric. If all metrics are absent, lines `76-79` report
that retrieval and generation quality are stable. `JsonFieldCodec.java:28-87`
also turns malformed stored JSON into empty lists, empty maps, or `null`, so
corrupt evaluation evidence feeds this fail-open path instead of invalidating
the run.

The citation accuracy and faithfulness labels at
`KnowledgeBaseDiagnosisService.java:49-54` are heuristic legacy metrics, not
semantic verification. They cannot be used as release evidence for factual or
citation correctness.

### P1: the Batch runner can exceed its configured concurrency and duplicate work

`BatchItemIngestionRunner.java:45-67` reads pending rows, separately counts
queued/processing rows, and then submits without a transaction, row claim, CAS,
lease, or unique dispatch identity. Concurrent refill calls can select the same
item. There is also an exact zero-slot error: even when `slots == 0`, the loop
submits one item before the `submitted >= slots` check. Tests do not run two
refillers against one database or kill/reopen around the submit boundary.

`UploadBatchService.java:200-249` marks an item `QUEUED`, invokes the ingestion
submission, then stores the returned task ID. Those effects do not form one
recoverable transaction with the downstream work, so failure between them can
leave duplicate or ownerless ingestion.

### P1: Batch staging has no owned, bounded, crash-safe lifecycle

`BatchStagingService.java:25-60` writes directly to the configured directory,
persists an absolute/path-like string in MySQL, later trusts that string, and
uses `readAllBytes`. It does not retain and revalidate an owned handle, verify
the stored bytes against the recorded hash before replay, impose a read bound,
publish atomically, fsync, or clean files on success/cancel/rollback. File I/O
also occurs inside `UploadBatchService.createBatchUpload`'s database transaction
at `80-125`; a rollback can leave untracked source bytes.

### P1: the benchmark and vector-health evidence is not trustworthy

`VectorStoreBenchmarkRunner.java:84-125` constructs a real
`DocumentEmbeddingService` and calls `embedDocument` for both sides. The
benchmark therefore mutates embedding/cache/vector state instead of evaluating
one frozen snapshot. `VectorStoreBenchmarkReportWriter.java:51-60` publishes
JSON and Markdown non-atomically, while `:68-103` and `:168-208` record mutable
environment paths, provider/model labels, Qdrant URL, and collection values but
no canonical corpus/pipeline/config digest.

For an ALL-scope audit, `VectorConsistencyAuditService.java:134-150` reconstructs
each vector with an empty embedding and a non-empty recorded dimension;
`:254-263` consequently reports systematic dimension mismatches. Conversely,
collection-scoped loading filters by collection at `127-133` before
cross-collection detection at `215-229`, making the intended pollution check
incapable of observing results excluded by that filter. The mock tests do not
replace these contradictions with a real Exact/Qdrant corruption, dry-run,
backup, repair, and receipt loop.

## Closed-loop assessment

The legacy tests exercise calculators, DTO mapping, repository behavior,
mocked retrieval, report formatting, and disabled-runner configuration. The two
test resources are small development fixtures, not a signed corpus or a clean
reproduction bundle. No test proves fixed source bytes plus one retrieval
snapshot plus one provider/pipeline fingerprint across process restart; no test
proves bounded cost, cancellation, crash recovery, real-browser operation,
concurrent Batch claiming, staging cleanup, or safe vector repair.

Accordingly:

- `MW-EVL-001`, `MW-BAT-001`, and `MW-HLT-001` stay `DROP` for Java code and
  `LATER_FROM_ZERO` as product ideas.
- Legacy datasets, runs, reports, batch rows, staging paths, audit rows, jobs,
  vectors, and derived state do not enter executable Go state.
- No Evaluation, Batch, or Knowledge Health table, job, route, page, feature
  flag, or provider purpose is created by this review.

## Current Go boundary

`v2/openapi/v1/contract.go:27-30` already forbids Evaluation, Agent, Memory,
Embedding, Rerank, Notification, Reindex, and Vector segments across production
packages, migrations, tables, routes, and commands. Keep that executable
absence gate. Batch is absent from the current Go package/schema/route/UI graph,
but its names are not yet part of the forbidden-segment list; adding an explicit
`batch`/`batches` negative gate is a small follow-up hardening choice, not a
reason to add a Batch implementation.

The current Go RAG/Ollama/Conversation/WebUI remains the core path. Evaluation
must not become a second answer pipeline or a way to relabel structural
citations as semantic verification. The real browser release gate is specified
separately in [`go-browser-gate.md`](./go-browser-gate.md); until its independent
browser runner exists, UI-001/UI-002 remain NOT_IMPLEMENTED/BLOCKED.

## Executed checks

The Git-blob evidence verifier, the current Go OpenAPI negative-surface tests,
and `git diff --check` are commit gates. No Java evaluation, batch worker,
provider, vector backend, migration, or user data was executed.
