# WebUI immutable mutation-attempt closure

## Identity and scope

This offline review starts from committed `codex/go-rewrite` revision
`bb176c8a2bd7a57558b52055527f49765e88105e`. The linear candidate commits are:

1. `0956b7ce754b4b127db150412b32bd2d4dee62a6`
2. `cf38ccdf143807b2f256aad22c60b09bc974c798`
3. `3928a3a6fbae0b6a878969cb48874215a4b2e851`

Before this evidence record, the candidate changes exactly two files:

| File | Lines | SHA-256 |
| --- | ---: | --- |
| `v2/internal/webui/static/app.js` | 1440 | `74b8d1d5c2bac9accb59cbc29e752ac100b73f98955e008d88e03ac9e7767ae9` |
| `v2/internal/webui/webui_test.go` | 319 | `16f6c3c5f1c2522428bbcd6204c27cc86be25eea16e326c21b155f565b5c0a74` |

No RAG/provider behavior, OpenAPI, backup, migration, release, dependency, or
Java file is changed. The shared WebUI file's unrelated backup workflow was
not changed.

## Server contract checked

The existing local API requires one idempotency key for upload, collection
creation, conversation creation, and Ask. The durable implementation already
binds upload to its request/source hashes, collection identity to its
normalized name, conversation identity to its title, and Ask to a canonical
request hash including conversation, expected revision, question, and optional
collection scope. Exact replay returns the prior resource; conflicting reuse
is rejected. This slice therefore changes only the browser-side ownership of
the key and payload.

## Closed browser boundaries

- Upload, collection creation, and conversation creation each retain one
  frozen attempt containing the idempotency key and the exact sent payload.
  Controls which could mutate that payload remain disabled until a definite
  rejection or confirmed terminal result.
- Successful responses are validated before releasing an attempt. Collection
  and conversation projections must match the frozen name/title. Ask must
  match the frozen conversation, question, and scope. Malformed or mismatched
  success responses fail closed and retain the attempt.
- A definite rejection is recognized only by the local API's stable Problem
  codes: `INVALID_ARGUMENT`, `UNAUTHENTICATED`, `FORBIDDEN`, `NOT_FOUND`,
  `CONFLICT`, and `RESOURCE_LIMIT`. HTTP class alone is not proof. Network
  errors, unstructured errors, 408, 429, and 5xx retain the exact attempt.
- During an uncertain Ask, the visible question and scope are restored from
  the frozen attempt and disabled. Conversation selection, conversation
  creation, and deletion of the active conversation are also blocked. Retry
  sends the same serialized body and key; it cannot read edited controls.
- A validated pending Answer starts polling by its fixed answer ID before
  best-effort catalog refresh. If that refresh fails, the admitted attempt is
  neither released nor reclassified as a provider outcome failure.

## Deterministic evidence

All commands used the pinned Go 1.27.0 Windows amd64 toolchain, local module
cache, no model, no user Vault, and no external network.

| Gate | Result |
| --- | --- |
| bundled Node `--check internal/webui/static/app.js` | PASS |
| `go test ./internal/webui -count=10` | PASS |
| `go test ./internal/app ./internal/webui` | PASS |
| focused WebUI/App vet | PASS |
| durable upload/Ask/restart/idempotency focused tests, `count=10` | PASS |
| `go vet ./...` | PASS |
| `go test ./...` | INTEGRATION BLOCKED only by production source identity refresh |
| `scripts/ci.ps1` | same identity blocker after all ordinary packages pass |
| `scripts/verify-standalone.ps1` | same identity blocker; temporary copy removed |

The candidate changes a production embedded asset, so the independently owned
source identities must be refreshed during integration. The candidate
calculates the `mindweaver` source manifest as
`02249b481d0772f8f9824559830b9408eef99f8a80f5906bfd235188da6b9218`
instead of embedded `006c3d00b663d6ab6417935c50c501c508b8e26783a9367ef46397f6969573e2`;
the dual-PE union calculates
`2217b9d9a08c3f7f0e171b60fc98a5df4f91dc0134fbd1718fa76fa9cfe5b591`
instead of embedded
`967be95103379b1d20e4310b82cbbd9ced377bd989427f20e196a137820e7bce`.
This branch intentionally does not edit either owning manifest.

## Decision and non-claims

No P0 or P1 remains inside the immutable-attempt slice. Production source
identity refresh is an integration P1 for the owning slice, not a waived gate.

These Go tests inspect the embedded browser contract and exercise the existing
HTTP/durable replay path with synthetic fixtures. They do not constitute a
real-browser UI-001/UI-002 pass. Repository approval for fixed offline browser
and driver artifacts remains empty, so real-browser qualification remains
`BLOCKED`; no fake harness is promoted as product evidence.

Attempt state is intentionally process-memory browser state. Reloading the
page cannot recover a lost pre-response idempotency key; a durable pending
Answer is rediscovered by conversation state and blocks a second Ask, but this
slice does not claim cross-page-reload recovery for an unacknowledged request.
