# Go salvage review

- Date: 2026-08-23
- Scope: committed `v2/` plus the three unmerged candidate lines
- Governing decision: [ADR 0011](./adr/0011-salvage-first-vertical-slices.md)

## Executive decision

The current Go tree is not a product baseline. It is a walking skeleton followed
by several protocol-first experiments. The executable can print a version and
initialize/check configuration, but it cannot upload a document, open a
production SQLite database, run ingestion, search, call a model, or serve an
HTTP request. Passing package tests therefore proves only that the invented
state machines are internally consistent.

None of the following candidate commits is accepted as a unit:

| Candidate | Decision | Reason |
| --- | --- | --- |
| `238ea04` platform/data kernel | Reject whole commit | Adds 3,594 lines without a SQLite/Event/Job adapter. Six overlapping coordination fields and caller-constructed before/after snapshots are not justified for one local writer. |
| `049ebb4` purge and ingestion hardening | Reject whole commit | An unbounded command-receipt ledger and a fourteen-class purge commitment model future subsystems that are not in the core. |
| RAG/provider/security and abandoned migration WIP in `5a8d` | Reject whole worktree | Roughly 9,938 lines still provide no HTTP handler, provider client, or completed RAG path. The 1,483-line invocation protocol is larger than the missing user workflow it is meant to protect. |

Review decisions apply to code, not just packages. A named invariant below may
be reimplemented in a smaller vertical slice; that is not authorization to copy
the surrounding aggregate, state machine, fixture, or API schema.

## Evidence threshold

A Go component is retained only when it meets one of these tests:

1. it is already used by a running product entry point and has failure-path
   tests at the real persistence or network boundary;
2. it is a small, dependency-free primitive whose behavior is already required
   by the next vertical slice; or
3. it is isolated qualification evidence and cannot be mistaken for production
   implementation.

Pure protocol coverage, Golden snapshots of invented aggregates, endpoint
counts, and compatibility with unfinished Java behavior do not meet the
threshold.

## Committed tree decisions

| Surface | Decision | What survives | Replacement gate |
| --- | --- | --- | --- |
| `go.mod`, build scripts, `platform/version` | `SIMPLIFY_NOW` | Standalone Go module, reproducible build, embedded version | Clean-checkout build remains green after every slice |
| `spikes/sqlite` | qualification evidence only | CGO-free Windows measurements for `ncruces/go-sqlite3`, including WAL, FTS5, cancellation, disk-full, backup and process-kill probes | Production store must independently pass integration and recovery tests |
| `platform/clock.go` | `SIMPLIFY_NOW` | A tiny injectable clock where a real persisted timeout needs it | At least one production consumer |
| `platform/id.go` | `SIMPLIFY_NOW` | Opaque UUID parsing/generation | Use a maintained UUID implementation; database sequence/cursor supplies ordering |
| `platform/config/**` | `CORE_REBUILD_FROM_ZERO` | Explicit Vault location and only options actually exposed by the product | Start the application with a fresh and existing Vault; reject unsafe locations |
| `platform/apperror/**` | `SIMPLIFY_NOW` | Safe internal error chain and stable code | One mapping table to the small HTTP Problem surface |
| `platform/event/**` | `DROP` | No generic envelope | Add a typed SQLite change log only when a real cross-restart UI stream requires it |
| `job/contracts.go` | `CORE_REBUILD_FROM_ZERO` | Durable local background work, bounded retry, cancellation, lease takeover | Real SQLite store with one `lease_token` CAS and two-connection/restart tests |
| `internal/document/**` | `DROP` code; `SIMPLIFY_NOW` invariants | Source/content hash, extractor version, and failed rebuild cannot replace a usable revision | Upload-to-search integration through SQLite and Blob storage |
| `internal/collection/**` | `DROP` code; `CORE_REQUIREMENT_ONLY` | True many-to-many membership; an explicitly empty scope returns no results | Normalized SQL tables and public search test |
| `internal/ingestion/**` | `DROP` | No separate execution state machine | One actual `INGEST_DOCUMENT` job handler with a bounded payload/checkpoint |
| `internal/indexing/**` | `DROP` code; `SIMPLIFY_NOW` invariant | New derived rows become visible atomically for one document; failure leaves the prior revision searchable | SQLite transaction and two-document regression test |
| `internal/provider/capability.go` | `DROP` then rebuild small | Only capabilities exercised by a real adapter | Minimal `ChatClient`; add streaming/embedding only with a consumer |
| `internal/provider/egress.go` | `CORE_REBUILD_FROM_ZERO` | Endpoint validation and address classification ideas only | Actual transport resolves once, dials the approved address, sets TLS SNI, ignores ambient proxy, and reauthorizes redirects |
| `internal/provider/invocation.go` | `DROP` | Do not auto-replay a request after an uncertain send | Message row uses `pending/completed/failed`; a manual retry creates a new message |
| `internal/transport/problem.go` | `SIMPLIFY_NOW` | RFC 9457 shape with stable `code` and `requestId` | Codes exist only for running handlers and are generated from one fixed mapping |
| `openapi/v1/**` | `DROP` then rebuild incrementally | Versioned `/api/v1` namespace | A path enters the document only after its handler and public behavior test exist |

All current document, collection, ingestion and generation Golden files are
deleted with their protocol aggregates. They freeze internal representations,
not user-observable behavior.

## Unmerged candidate decisions

### Platform candidate `238ea04`

Do not copy `ExecutionStamp`, `WriterEpoch`, `FencingToken`,
`TargetOperationEpoch`, `Resolution`, caller-created event envelopes, the
definition registry, generic CAS package, strict JSON walker, or the custom
UUIDv7 implementation. A local SQLite job needs a row version and one opaque
lease token. A remote provider cannot enforce a local fencing token, so extra
epochs do not prevent duplicate external effects.

The only concepts to reimplement are:

- a late job worker cannot commit after lease takeover;
- bounded retry and cancellation converge to a visible terminal state;
- configuration and HTTP JSON are decoded into typed bounded inputs; and
- secrets never enter formatted errors or logs.

### Knowledge candidate `049ebb4`

Do not copy aggregate-wide command receipts, revision/operation-epoch
coordination, canonical purge commitments, or typed proof ledgers. The first
release uses soft delete for immediate exclusion. Permanent deletion removes
owned database rows transactionally and lets a reference-aware orphan scan
remove immutable blobs. It reports partial failure honestly; it does not claim
cryptographic or lineage-complete erasure.

Retained behavior is limited to:

- failed ingestion/rebuild never hides a previously usable revision;
- a deleted document is absent from SQL and FTS results; and
- a deletion operation cannot report success while a known in-scope database
  or blob removal failed.

### RAG/provider/security and abandoned migration WIP

Delete the invocation proof hierarchy, capability registry, retrieval snapshot
ledger, answer-verification aggregate, speculative OpenAPI changes, and generic
full/incremental migration format. The Java exporter, neutral package, Go
importer, migration CLI, and legacy-import schema are discarded under ADR 0014.

One small implementation fragment is eligible for manual extraction after
review: a secret-token type that refuses ordinary formatting and JSON
serialization. Bootstrap, Host, Origin, CSRF and Fetch Metadata checks are
reimplemented around one process-local session bound to `127.0.0.1`; trusted
network mode and persistent session generations are excluded.

Citation structure is rebuilt as a server-owned
`SourceRef{chunkID, documentID, title, excerpt}` and may point only at a chunk
actually included in that answer's model context. Structural citation checks
must never be labelled semantic verification.

The product creates a fresh Vault and accepts content only through ordinary
bounded Go upload. Go's own SQLite schema upgrades and new-Vault backup/restore
remain required.

## First release surface

The first implementation proves one chain before adding another abstraction:

```text
loopback HTTP upload
  -> bounded staging + SHA-256 immutable blob
  -> SQLite document + INGEST_DOCUMENT job
  -> deterministic text extraction and chunking
  -> document revision + chunks + FTS5 in one activation transaction
  -> scoped search
  -> optional chat using exactly those final chunks and server-owned sources
```

Initial production packages are limited to:

- `internal/store/sqlite`
- `internal/blob`
- `internal/ingest`
- `internal/search`
- `internal/provider/openai_compatible`
- `internal/httpapi`
- `internal/app`

The initial schema is limited to `documents`, `document_revisions`, `chunks`,
`chunks_fts`, `jobs`, `collections`, and
`collection_documents`. `chats`, `messages`, and `message_sources` are added
only with the chat slice.

## User-observable retention gates

The protocol tests being removed are replaced by tests of these outcomes:

1. upload text, observe the job finish, and find its content through the public
   search endpoint;
2. stop and reopen the process and obtain the same search result;
3. extraction failure publishes no half-active revision;
4. rebuild failure leaves the previous revision searchable;
5. an empty collection selection never broadens to the entire Vault;
6. delete removes SQL/FTS visibility and a referenced blob is never collected;
7. a fake provider completes one answer whose sources are a subset of the exact
   chunks sent to it; and
8. hostile DNS, redirect, proxy, timeout and cancellation tests exercise the
   real provider transport.

## Explicitly later or absent

Agent, Agent Profile, Memory, Batch, Notification, Evaluation, Qdrant, vector
retrieval, hybrid fusion, reranking, query understanding, retrieval trace,
permanent-delete proofs, generic external-call forensics, LAN exposure,
accounts, collaboration and cloud synchronization are
not dormant first-release implementations. They are absent and must earn a new
zero-to-one design and promotion ADR later. Java/MySQL data conversion is
`DROP`, not a dormant later slice.
