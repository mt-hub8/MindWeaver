# Go RAG / Ollama boundary qualification

## Identity and scope

This offline qualification is based on committed `codex/go-rewrite` revision
`d61a158afdc7c86914c70a847ce4e9d6fe686517`. The test-only change is
`2ad81d17b0d91a6b0a839710280cf89de7eb5811` and modifies exactly these files:

- `v2/internal/ollama/client_test.go`
- `v2/internal/rag/service_test.go`
- `v2/internal/app/rag_product_test.go`

No production, OpenAPI, WebUI, backup, release, migration, schema, or dependency
file changes are part of this qualification.

The production inputs exercised by the tests are frozen below. Line counts
include blank lines and comments; hashes are lowercase SHA-256 of the bytes at
the base revision.

| File | Lines | SHA-256 |
| --- | ---: | --- |
| `v2/internal/ollama/client.go` | 489 | `eccd64f477233be24af844dde84d7f2c6727ba07ba325d54824abc42d428c08a` |
| `v2/internal/rag/service.go` | 527 | `79e3c2aa3c166525c15c8b2a625263a8de619981f32bfaf270fa61c7a87fb32e` |
| `v2/internal/app/rag_runtime.go` | 142 | `c7a04a646e609281e567b977f8fdac7208cfa6a2c676845ddfe2d1b7d08970f6` |
| `v2/internal/app/app.go` | 428 | `87545c6324fd00d4da1be1a335dbd2b704dae4fc19eabcf78cf3ebbde6dcda0b` |
| `v2/internal/app/api_rag.go` | 564 | `3d636d7abaa70db2eb9645b4e6e2446a5df4eb0d6b758fd4aa6671253adc2416` |
| `v2/internal/app/json.go` | 192 | `b171a395e2837068c70b7d472c728d16f7cfe7c89d9c9c0a7c7a1eac679c6ae0` |

## Qualified boundaries

### Provider request admission and cancellation

- A prompt of exactly `ollama.MaxPromptBytes` (64 KiB) reaches the loopback
  provider once and succeeds. A prompt one byte larger returns
  `ErrRequestTooLarge` without a second network call.
- Caller cancellation while the client is waiting for response headers
  interrupts the production HTTP request. The returned error is
  `context.Canceled` and does not contain the fixed prompt canary.

### Durable uncertainty and exact replay

- Cancellation after the durable invocation boundary converges to a failed
  answer with `OUTCOME_UNCERTAIN` rather than retrying the provider.
- Closing and reopening SQLite, then replaying the exact request, returns the
  same answer identifier and preserves a provider call count of one.
- The existing post-response completion-failure seam now also closes and
  reopens SQLite before exact replay; it retains the same uncertain answer and
  does not regenerate.

### Ask body admission before reservation

- An Ask JSON body larger than the 16 KiB application limit is rejected with
  HTTP 400 before provider I/O and without echoing the body canary.
- The same idempotency key remains usable by a later valid request. That request
  completes with exactly one provider call, and its exact replay returns the
  identical response without another call. This proves that the oversized body
  did not create a durable idempotency reservation.

## Executed offline gates

All commands used the pinned Go 1.27.0 Windows amd64 toolchain with no model,
user Vault, or external network access.

| Gate | Result |
| --- | --- |
| Ollama exact-bound/header-cancel focused test, `count=10` | PASS |
| RAG uncertainty/reopen exact-replay focused test, `count=10` | PASS |
| App Ask body-admission product test, `count=10` | PASS |
| `go test ./internal/ollama ./internal/rag ./internal/app` | PASS |
| `go vet ./internal/ollama ./internal/rag ./internal/app` | PASS |
| `go vet ./...` | PASS |
| `go test ./...` | BASELINE BLOCKED |
| `scripts/ci.ps1` | BASELINE BLOCKED |
| `scripts/verify-standalone.ps1` | BASELINE BLOCKED |

The three blocked gates fail at the same pre-existing committed-base assertion:
`openapi/v1.TestEmbeddedContractMatchesProduction` calculates the `mindweaver`
transitive source-manifest SHA-256 as
`7de68fba33368f4cfa74a44cb27a0c5add95c688f70c0c77b939bbb21d825dca`,
while the embedded contract contains
`325dfec05db7f7744c6aaf4de792d80efaa6b458a0b850a635d3c41e3596e099`.
The same assertion and hashes fail on the unmodified base. Every other package
reported by the full test run passes. This test-only slice does not modify the
contract or its production source closure.

Both CI scripts removed their bounded temporary trees after the blocked run;
no build or standalone residue remained.

## Decision and non-claims

P0 and P1 are zero within this test-only slice. Its focused production-path
boundaries are qualified for selective integration after the owning baseline
manifest mismatch is resolved.

The tests use fixed literal-loopback providers and temporary synthetic SQLite
fixtures. They do not claim compatibility with a real Ollama installation,
user data, or forced operating-system process termination. No automatic
provider replay is introduced or authorized by these tests.
