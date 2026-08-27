# REL-001 current-main qualification boundary

Status: **NOT_IMPLEMENTED**

- Qualification-source baseline: `37ca406a23fc64a8dd8a5f6ea3aa823b64e49fb1`
- Latest integrated ordinary-gate baseline:
  `71bd8049e70e5a0c1cc16df89f48870f72cdb426`
- Blob operation-sequence fuzz target:
  `57b469268d1505e48cee38298d8f1aa6e7721c7a`
- Recorded: 2026-08-27 (Asia/Shanghai)
- Scope: current committed developer signals only; no production, test, script,
  vendor, identity, or release-gate behavior is changed by this evidence.

REL-001 is conjunctive: race, fuzz, fault, performance, and soak gates must all
pass for the CORE scope. The repository now contains useful developer signals,
but it does not contain the complete accepted release qualification bundle.
This evidence therefore does not promote the row to `IMPLEMENTED` or `PASS`.

## KEEP: reproducible developer signals

The owner-local fuzz closure discovers and runs exactly five production fuzz
targets with the pinned Go toolchain, real mutation, a one-second fuzz duration,
one worker, bounded output, and network-disabled module resolution:

- `internal/blob/FuzzParseIDCanonical`;
- `internal/blob/FuzzStoreOperationSequence`;
- `internal/ingest/FuzzChunkTextDeterministicAndBounded`;
- `internal/ingest/FuzzReadTextCanonicalAndBounded`;
- `internal/pdfextract/protocol/FuzzDecodeResultRequiresCanonicalFrame`.

The Blob sequence target drives the real filesystem-backed
Prepare/Publish/Abort/Import/Delete/Cleanup/Reopen transitions and checks the
exact object and staging state after every operation. A separate ten-second
probe completed 493 executions and retained three new coverage inputs. It does
not introduce a production hook, journal, or second state machine.

The five-target focused run passed in 13.78 seconds. The three-second production
short soak also passed with 81 upload/ingest/search/replay/trash/purge cycles and
81 invariant checks. It observed at most four regular Vault files and 4,645,016
bytes. This is a bounded smoke signal, not a long-soak result.

The three production-boundary benchmarks remain useful for local comparisons:

| Benchmark | Current observation |
| --- | ---: |
| durable Blob publication | 3.311 ms/op, 4.95 MB/s |
| bounded upload plus ingestion | 19.180 ms/op, 0.85 MB/s |
| literal SQLite FTS over the frozen 32-document corpus | 0.763 ms/op |

These are one developer-machine observations from `-benchtime=10x`; no accepted
machine, variance, latency, memory, or capacity threshold is attached to them.
They cannot enter a release decision as an SLO result.

Deterministic SQLite WAL/busy, `SQLITE_FULL`, corruption, FTS-divergence, and
forced-exit tests; Blob write/short-write/sync/cancellation/publication tests;
and the selected runtime, ingestion, Answer, and Blob kill/reopen tests are also
KEEP developer fault signals. Their executable sources include:

- `v2/internal/store/sqlite/qualification_fault_test.go`;
- `v2/internal/blob/{prepare_fault_test.go,publish_context_test.go}`;
- `v2/qualification/{runtime,knowledge}`.

They prove their named deterministic checkpoints. They are not a substitute
for a randomized release campaign.

## Open release gates

The Windows release CI deliberately freezes `CGO_ENABLED=0`. On this baseline,
`go test -race` fails closed with `-race requires cgo`. No approved pinned C
compiler and CGO-enabled qualification lane exists. The **race facet is
BLOCKED**; the shipped CGO-disabled build must not be silently changed to make a
qualification command run.

The current machine does contain Visual Studio Build Tools 18.4.3 and MSVC
`cl.exe` 19.50.35728.0 (SHA-256
`194ddf4aafcb74452218a982309a97de30e0adb33edf4af02904ee107213e782`),
but no GCC or Clang tool. A real `CGO_ENABLED=1 CC=cl go test -race` probe reaches
`runtime/cgo` and fails because MSVC rejects the Go cgo driver flag `/Werror`.
Ambient MSVC therefore does not close the race lane and is not silently treated
as an approved compatible compiler.

The following required release evidence is also absent:

- a 24–48 hour supported-Windows soak under the accepted machine/EDR profile;
- at least 1,000 randomized process kills around acknowledged and
  unacknowledged operation identities;
- 10k/100k representative capacity, cold/hot latency, RSS, and versioned product
  SLO budgets;
- an accepted duration/corpus and multi-component operation-sequence campaign
  beyond the one-second Blob state-machine target.

The three-second soak, selected forced exits, benchmark output, `go vet`, and a
self-reported JSON result must not be relabeled as those missing gates.
REL-001 therefore remains `NOT_IMPLEMENTED`, rather than treating the blocked
race facet as proof that the rest of the bundle has been implemented.

## Current-main repair verification

The earlier unrelated current-main qualification regressions are closed:

- `64d001e367bb0342d4c933f765fe0c0383df38bd` refreshed the reviewed
  source/dual-PE identities. `TestEmbeddedContractMatchesProduction` and
  `TestWindowsAMD64ShippedDualPEClosure` pass on this baseline.
- `37ca406a23fc64a8dd8a5f6ea3aa823b64e49fb1` routes only the exact authorized
  knowledge child selectors before module/PDF-helper setup.
  `TestBLOB001PublishedOrphanChildRejectsUntrustedEnvironment` passes with
  `-count=10`.

Those repairs restore their own qualification signals; they do not close any
of the REL-001 release-scale gaps above.

## Ordinary CI is green, not REL-001 qualification

The controller's clean current-main verification at
`71bd8049e70e5a0c1cc16df89f48870f72cdb426` passed both
`v2/scripts/ci.ps1` and tracked-only `v2/scripts/verify-standalone.ps1`. The
standalone gate archives the committed `v2` tree without Git metadata and runs
the ordinary offline CI again from that extracted tree.

This closes the earlier ordinary-CI regression and proves the committed tests,
`go vet`, offline builds, and tracked-only extraction on that exact revision.
Neither script runs `go test -race`, the three benchmarks, an extended soak, a
1,000-kill campaign, or a release-machine performance qualification. Their
green result is therefore KEEP integration evidence and does not promote
REL-001 beyond `NOT_IMPLEMENTED`.

## Reproduction

From `v2/` with the repository-selected Go 1.27.0 toolchain and the frozen
offline environment:

```powershell
go test ./qualification/reliability -run '^(TestCoreFuzzTargetClosureAndExecution|TestShortSoakProductionInvariants|TestShortSoakConfigIsFrozenAndBounded)$' -count=1 -v
go test ./qualification/reliability -run '^$' -bench '^(BenchmarkBlobDurablePublication|BenchmarkWorkbenchUploadAndIngest|BenchmarkSQLiteFTSSearch)$' -benchtime=10x -count=1
go test ./qualification/knowledge -run '^TestBLOB001PublishedOrphanChildRejectsUntrustedEnvironment$' -count=10
go test ./openapi/v1 ./qualification/production -run '^(TestEmbeddedContractMatchesProduction|TestWindowsAMD64ShippedDualPEClosure)$' -count=1
```

All four positive commands passed. The deliberate feasibility probe

```powershell
$env:CGO_ENABLED = '0'
go test -race ./internal/blob -run '^TestImportDeduplicatesContent$' -count=1
```

failed with `go: -race requires cgo; enable cgo by setting CGO_ENABLED=1`, as
expected from the frozen release toolchain. This is evidence of the open race
gate, not a passing test.
