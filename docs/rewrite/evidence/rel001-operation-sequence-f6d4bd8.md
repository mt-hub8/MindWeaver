# REL-001 knowledge lifecycle operation-sequence addendum

Status: **NOT_IMPLEMENTED**

- Code commit: `f6d4bd858a9bae1304039b06cd1f276b1087e0fe`
- Parent pre-package evidence commit:
  `592339e1d6eef911e199f6c1c713ec03ece0d912`
- Recorded: 2026-08-28 (Asia/Shanghai)
- Scope: a bounded developer fuzz signal for the existing CORE knowledge
  lifecycle; it is not a release-scale reliability result.

This addendum preserves the historical evidence in
`rel001-current-main-37ca406.md` and records the sixth owner-local fuzz target
for production code added on the current main line. It does not rewrite the older
report's five-target provenance or promote `REL-001`.

## New bounded sequence target

`internal/lifecycle/FuzzKnowledgeLifecycleOperationSequence` drives the real
SQLite, Blob, workbench, lifecycle, collection, ingestion and FTS boundaries.
Each input is a bounded `(uint64 plan, uint8 steps)` pair with at most 12 steps,
16 possible actions, two modeled documents and one deliberately shared Blob.
Three checked-in seeds cover normal, stale and idempotent paths.

The actions are upload, conflicting upload, run-one, cancel, current/stale
retry, current/stale trash, current/stale restore, current/stale purge,
membership add/remove, reopen and Blob-candidate sweep. A separate shadow model
predicts each transition before the production call. The queue oracle uses the
current production-observable scheduling tuple `RunAfter`, `CreatedAt`, then
`JobID`; retry legitimately changes `RunAfter` and the model captures the new
value before predicting subsequent queue order.

After every step the target checks document, job, membership, referenced Blob,
candidate, global search and collection search state. Destructive, reopen and
sweep steps, plus the terminal checkpoint, additionally run SQLite integrity,
raw read-only exact table counts, filesystem exact-tree checks and staging
checks. Shared-reference and last-reference purge behavior, plus
reference-aware candidate sweep, are included.

Randomized purge-delete failure injection was deliberately removed from this
target. `TestPurgeFailureIsDurableVisibleAndRetryable` already owns that exact
deterministic failure boundary; duplicating it inside a broad state machine
would add model complexity without proving a new product state.

## Exact fuzz closure and observed runs

`qualification/reliability/TestCoreFuzzTargetClosureAndExecution` discovers the
owner-local production fuzz functions from source and now requires exactly six:

- `internal/blob/FuzzParseIDCanonical`;
- `internal/blob/FuzzStoreOperationSequence`;
- `internal/ingest/FuzzChunkTextDeterministicAndBounded`;
- `internal/ingest/FuzzReadTextCanonicalAndBounded`;
- `internal/lifecycle/FuzzKnowledgeLifecycleOperationSequence`;
- `internal/pdfextract/protocol/FuzzDecodeResultRequiresCanonicalFrame`.

The closure uses one private empty `GOCACHE`, preventing ambient developer fuzz
corpora from turning the fixed mutation proof into a false result. The five
existing targets retain a one-second mutation run; the new lifecycle target
uses deterministic `32x`. Output, process duration, module resolution and
parallelism remain bounded.

Independent current-tree runs passed:

- the three lifecycle seeds with `-count=10` (3.391 seconds);
- `internal/blob`, `internal/store/sqlite`, `internal/workbench` and
  `internal/lifecycle` package tests;
- vet for the affected packages;
- the exact six-target closure (39.16 seconds total; lifecycle 4.52 seconds),
  with every target reporting real fuzz execution and `PASS`.

## Deliberate non-claims

This target is single-process and bounded. It does not prove a race-enabled
lane, concurrent scheduler interleavings, process termination, physical-media
faults, an accepted duration or corpus, 24-48 hour soak, 1,000 randomized
kills, or 10k/100k capacity and product SLOs. The pinned shipped configuration
also remains `CGO_ENABLED=0`, so the approved race lane is still absent.

The sequence target therefore closes one previously missing developer signal,
but the conjunctive `REL-001` release gate remains `NOT_IMPLEMENTED`.

## Reproduction

From `v2/` with the repository-selected Go 1.27.0 toolchain and the frozen
offline module environment:

```powershell
go test ./internal/lifecycle -run '^FuzzKnowledgeLifecycleOperationSequence$' -count=10
go test ./internal/blob ./internal/store/sqlite ./internal/workbench ./internal/lifecycle -count=1
go vet ./internal/blob ./internal/store/sqlite ./internal/workbench ./internal/lifecycle ./qualification/reliability
go test ./qualification/reliability -run '^TestCoreFuzzTargetClosureAndExecution$' -count=1 -v
```
