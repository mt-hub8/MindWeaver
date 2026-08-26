# ADR 0012: Lean local core and observable recovery

- Status: Accepted
- Date: 2026-08-23
- Supersedes: the generic job/event protocol in ADR 0004, the durable
  invocation ledger in ADR 0005, the persisted retrieval-snapshot and semantic
  verification model in ADR 0006, and (historically) the broad migration scope
  in ADR 0008; ADR 0014 now supersedes that migration decision in full

## Context

The first Go implementation modeled distributed coordination, provider-call
forensics, immutable retrieval traces, and compatibility formats before it had
a SQLite repository, HTTP handler, provider client, or upload-to-search path.
Those models passed their own tests but did not close a user workflow. For a
single-user, one-process local workbench they also duplicated the same facts
across Job, Ingestion, Generation, Invocation, Event, and Answer aggregates.

The rewrite needs crash honesty and security, but complexity is not evidence of
either. Safety must be enforced by the component that owns the real boundary:
SQLite transactions for local state, filesystem primitives for source blobs,
and an actual HTTP transport for provider egress.

## Decision

### Local jobs and UI progress

The first release has one SQLite `jobs` table and these states only:
`QUEUED`, `RUNNING`, `SUCCEEDED`, `FAILED`, and `CANCELLED`. Delayed retry is a
queued job with `run_after`. A running claim has one opaque `lease_token` and a
deadline. Renew, complete, fail, and cancellation convergence use a conditional
update matching job ID, `RUNNING`, and the lease token. A late worker therefore
cannot commit after takeover.

`attempt` and `max_attempts` bound retry. There is no separate WriterEpoch,
FencingToken, OperationEpoch, Attempt aggregate, Resolution aggregate,
definition registry, generic checkpoint, caller-created before/after snapshot,
or generic event envelope. A concrete ingestion handler may persist a small
domain checkpoint only after it demonstrates a resumable boundary.

The browser may poll document and job resources in the first vertical slice. A
typed SQLite change log and resumable SSE are introduced only when a real UI
consumer proves polling insufficient. State changes do not wait for a generic
event system.

### Documents and retrieval

SQLite owns documents, revisions, chunks, collection membership, lifecycle,
and which revision is active. Immutable content-addressed files own newly ingested
source bytes. New chunks and FTS rows are built for one document revision and
become visible through one SQLite activation transaction; a failed build leaves
the prior active revision searchable.

The core retriever is one bounded FTS5 query over active, non-trashed documents.
Collection scope is expressed by normalized SQL joins, and an explicit empty or
unknown collection selection returns no results. Vector search, hybrid fusion,
reranking, query expansion, and persisted candidate traces are absent.

Search and Ask call the same query function. Ask persists only the final source
chunk IDs and server-generated display metadata used for that message. It does
not persist every candidate, rejection, score stage, or a generic
`RetrievalSnapshot` aggregate.

A citation is structurally valid only when it refers to one of those final
source chunks. This proves provenance, not semantic entailment. The product
must not label structural checks as answer verification. With no usable context
or a failed provider call, it returns an honest controlled error/refusal instead
of a fabricated supported answer.

### Provider calls

ADR 0005's network controls remain binding: configured authority is explicit;
cloud endpoints use HTTPS; local plaintext endpoints are loopback-only; the
transport resolves once, dials an approved address, preserves TLS SNI, ignores
ambient proxies, bounds every phase, and does not follow an unapproved redirect.

The generic provider capability registry and four-stage durable invocation
artifact protocol are removed. A concrete adapter exposes only behavior it
implements. A chat message is `pending`, `completed`, or `failed`, with a stable
safe failure code. If the process loses certainty after sending a request, that
message fails as outcome-uncertain and is never automatically replayed. A user
retry creates a new message. The provider is not claimed to enforce local lease
or fencing metadata.

### Deletion and historical migration decision

Trash immediately removes a document from retrieval and remains reversible.
Permanent deletion transactionally removes known in-scope SQLite/FTS ownership;
a reference-aware orphan pass removes unreferenced source blobs. Any known
failure is reported as failure. The first release does not issue a signed
erasure receipt, retain an unbounded command ledger, or claim deletion from
future subsystems that do not exist.

The following migration decision is superseded in full by ADR 0014. The read-only
legacy exporter includes canonical documents that still exist, verified
collection membership, consistent lifecycle, and non-secret provider intent.
Derived chunks/vectors/caches, executable jobs, Agent/Memory/Batch/Evaluation
state, and legacy ciphertext do not become live Go state. Selected task/history
material may enter a checksummed non-executable archive. There is no generic
incremental interchange framework in the first release.

The current product instead creates a fresh Vault and exposes no legacy import
surface. Ordinary Go SQLite schema upgrades are not legacy-data migration and
remain required.

## Consequences

- The mainline protocol aggregates, Golden snapshots, generic event package,
  speculative OpenAPI, and candidate branch protocol expansions are deleted.
- OpenAPI is rebuilt endpoint by endpoint from running handlers.
- A write endpoint receives idempotency or version preconditions only when
  replay or concurrent editing is a demonstrated user boundary; it is not a
  blanket rule for every local mutation.
- Advanced capabilities are absent rather than compiled behind flags.
- A future ADR may add an event log, provider invocation evidence, vector
  retrieval, or richer deletion proof after a concrete consumer and failure
  model exist.

## Verification gates

The replacement architecture is accepted through observable integration tests:

1. two database connections cannot claim or complete one job with stale lease
   ownership;
2. process close/reopen recovers expired work and preserves FTS results;
3. upload, Blob publication, ingestion activation, and search cross their real
   filesystem and SQLite boundaries;
4. failed extraction or rebuild publishes no half-active revision;
5. empty collection scope, trash, and delete never leak content into search;
6. the real provider transport passes hostile resolver, redirect, proxy,
   timeout, cancellation, and response-size tests; and
7. a fake provider answer can reference only the exact final chunks supplied to
   that call.
