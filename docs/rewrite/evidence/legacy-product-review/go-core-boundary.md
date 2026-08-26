# Current Go RAG / Ollama / Conversation / WebUI boundary

## Frozen surface

- Baseline: `9bae471a113e89de87332a07994933ad62f149cf`
- Machine-readable evidence: [`go-core-boundary.csv`](./go-core-boundary.csv)
- Verification: `pwsh -NoProfile -File docs/rewrite/evidence/legacy-product-review/verify.ps1 -Manifest docs/rewrite/evidence/legacy-product-review/go-core-boundary.csv`

This is comparison evidence for the independently designed Go CORE. It is not
a Java salvage list and contains no legacy migration surface.

| Boundary | Files | Decision |
| --- | ---: | --- |
| Stable cursor contract | 2 | `KEEP` |
| Literal-loopback Ollama client | 2 | `KEEP` |
| Conversation / answer Store and migration 004 | 3 | `KEEP` |
| RAG application and public API wiring | 6 | `HARDEN` |
| RAG runtime and structural citation validation | 2 | `HARDEN` |
| Embedded WebUI | 4 | `HARDEN` |
| OpenAPI and CORE-surface contract | 5 | `HARDEN` |
| **Total** | **24** | **7 KEEP / 17 HARDEN** |

There is no standalone generic `internal/conversation` framework. Conversation
state is a closed vertical through migration 004, the SQLite adapter, the
application/API, and the embedded UI. Adding a dormant abstraction would not
improve this product boundary.

## KEEP

The Ollama client accepts only one validated literal-loopback endpoint, dials
that address directly, disables ambient proxies and redirects, bounds requests
and responses, and exposes content-free failures. It is non-streaming Chat,
not a revived Python worker, generic Provider framework, embedding platform, or
Agent runtime.

The Store keeps durable conversations, messages, answer attempts, ordered
sources, provider-configuration identity, idempotency/revision data, and
reconciliation state. The cursor contract fails closed on malformed or stale
pagination state. These are active CORE invariants backed by offline tests.

## HARDEN

`internal/rag` validates citation syntax and membership in the exact retrieved
source set. It does **not** prove that a cited passage semantically supports a
claim, perform fact checking, or justify “verified citation” language. The Go
UI now says only that citation numbers point to material used for the current
answer, and `TestEmbeddedClientDoesNotClaimSemanticCitationVerification`
prevents the stronger wording from returning.

The real-browser gate remains open. Existing Go UI tests validate embedded
assets and client contracts, not browser behavior. The required Windows
executable/browser/fake-Ollama qualification is specified in
[`go-browser-gate.md`](./go-browser-gate.md) and remains `NOT_IMPLEMENTED` /
`BLOCKED` until a pinned offline browser artifact is approved and the actual
`mindweaver.exe` passes it.

OpenAPI must continue to match real handlers, problem/status mappings, and the
checked CORE surface. The production-surface contract explicitly rejects
Agent, Batch, Evaluation, KBHealth, Memory, Notification, embedding/reindex,
rerank, and vector package/route segments. Commit `9bae471` closes the missing
Batch/KBHealth absence checks; it does not create those features.

## DROP / absent

No Java RAG/provider code, Python worker, Java static page, Java-derived job or
vector state, generic Agent/Memory/Evaluation/Batch framework, or abandoned
legacy migration component belongs in this Go boundary. Future product work in
those areas requires a new zero-to-one decision and real consumer workflow.

## Qualification status

- Offline package/contract tests: executable now and required below.
- Semantic citation verification: deliberately not claimed.
- True browser UX/security/accessibility gate: `NOT_IMPLEMENTED`, therefore
  release evidence remains `BLOCKED` for that gate.
- Real Ollama and user data: not used by this review.
