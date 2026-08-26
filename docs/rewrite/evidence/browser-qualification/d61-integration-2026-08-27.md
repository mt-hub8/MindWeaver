# Browser qualification selective integration on d61

## Identity

This test/evidence-only integration starts from
`codex/rag-boundary-d61@58646cfbc9f3575106917c87f1462b0706f43d66`,
whose committed mainline base is
`d61a158afdc7c86914c70a847ce4e9d6fe686517`. The reviewed browser commits were
applied without conflict and in their original order:

| Reviewed commit | Integrated commit |
| --- | --- |
| `1af9c95813f4d7fe5b20f0dfb634a447692fcda1` | `f237d389c4c036db7bdcf81c2aca2e4a1119c955` |
| `87c25497710fefc3e0f5834b1e56ad470cbf72c2` | `795bbf1bd8993981ae02f7e35a957a7e9244172a` |
| `443c07ccc9831fe839ebbad32fe17fcbc8492327` | `395a58a7411e8b5391a97caf6267435bd25cd79c` |

The integrated browser tree is byte-identical to the reviewed final commit.
The earlier offline machine observation remains explicitly bound to its
recorded time and `sourceBase=e8cf70281eabc19ae8762cb311fb2bede5255df4`;
this integration does not rewrite or reinterpret that historical observation.

## Closed integration boundary

- The embedded approval still contains an empty `artifacts` array.
- The public runner returns `BLOCKED/BROWSER_ARTIFACT_NOT_APPROVED` before an
  artifact or product-process boundary. Supplying a future approved tuple still
  cannot reach PASS without a package-owned real process harness, which is not
  implemented.
- The controlled fake WebDriver/process harness remains permanently
  `BLOCKED/CONTROLLED_HARNESS_NOT_QUALIFIED` and cannot satisfy the private real
  qualification proof.
- Browser changes are limited to `v2/tests/browser/**`,
  `v2/scripts/test-browser.ps1`, and browser evidence. The current
  `v2/scripts/ci.ps1`, production packages, OpenAPI, and release files are byte
  unchanged from the d61 base.
- The runner has no non-standard dependency outside its two test packages and
  imports no production `internal`, command, OpenAPI, or release package.

This integration therefore does not create a production executable dependency,
release qualification, or CI PASS. UI-001 and UI-002 remain BLOCKED.

## Executed offline gates

All Go commands used the pinned Go 1.27.0 Windows amd64 toolchain with
`GOTOOLCHAIN=local`, `GOPROXY=off`, `GOSUMDB=off`, and readonly modules.

| Gate | Result |
| --- | --- |
| `go test ./tests/browser/... -count=10` | PASS |
| `go vet ./tests/browser/...` | PASS |
| `scripts/test-browser.ps1 -SelfTest`, 10 consecutive runs | PASS, blocked as designed |
| Browser runner dependency closure | PASS, production forbidden = 0 |
| Three RAG/Ollama focused tests, each `count=10` | PASS |
| `go vet ./...` | PASS |
| `go test ./...` | BASELINE BLOCKED |

The full test run includes both new browser packages and they pass. Its only
failure remains `openapi/v1.TestEmbeddedContractMatchesProduction`: the
calculated `mindweaver` transitive source-manifest SHA-256 is
`7de68fba33368f4cfa74a44cb27a0c5add95c688f70c0c77b939bbb21d825dca`,
while the committed contract contains
`325dfec05db7f7744c6aaf4de792d80efaa6b458a0b850a635d3c41e3596e099`.
The browser integration changes neither input and does not mask this existing
blocker.

## Decision

P0 and P1 are zero for the selective integration itself. Real UI qualification
remains blocked on repository-approved offline browser/driver artifacts and a
real Windows process sandbox/cleanup proof; no such evidence is claimed here.
