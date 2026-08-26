# MindWeaver Go rewrite execution charter

## Objective

Replace the Java implementation with a local-first Go application that can be
installed, operated, backed up, restored, upgraded, and removed without MySQL,
RabbitMQ, or Python workers. The Java tree is read-only historical product
evidence, not an implementation template or a supported user-data source.

The Go implementation is developed under `v2/` and must be independently
buildable from its first executable milestone. It will be extracted to a new
repository only after release qualification passes.

## Product boundary

- Single user and one active Vault per process.
- Windows 10/11 x64 is the first Tier-1 platform.
- Offline-first; Ollama is the default optional model runtime.
- V1 model integration is optional loopback Ollama; cloud credentials and
  provider expansion require a separately accepted slice.
- Text, Markdown, and text-based PDF are the initial document formats.
- No account, collaboration, cloud synchronization, or implicit LAN mode.

The first release is salvage-first, not a Java parity rewrite. Its product slice
is canonical documents and lifecycle, true M:N collections, controlled keyword
retrieval, Ask with refusal and same-context citations, optional loopback Ollama
with durable message outcomes, conversation, secure local UI/session, plaintext
Vault backup/restore, and versioned Go-to-Go schema upgrades. Agent,
Profile, Memory, Batch, Notification, Evaluation, Qdrant, reindex, advanced
retrieval/rerank/query understanding, cache/storage consoles, and advanced index
repair are absent until separately rebuilt from zero and accepted.

## Architectural invariants

1. SQLite is authoritative for first-release state, membership, and lifecycle.
2. Immutable blobs are authoritative for source bytes ingested by the Go product.
3. FTS and chunks are rebuildable first-release derivatives. Embeddings, vector
   indexes, and caches belong to later independently accepted slices.
4. Core background work uses one durable job protocol that prevents concurrent
   duplicate execution and makes retry, cancellation, restart, and final failure
   visible.
5. External I/O never runs inside a database transaction.
6. Core Search and Ask use the same scoped SQLite FTS path; later entrants must
   conform to its lifecycle and collection rules before promotion.
7. A citation can reference only a stable source chunk supplied for that answer.
8. No-hit and structurally invalid citation cases refuse or show an explicit
   limitation label; the product does not claim semantic truth verification.
9. Reindex is not a first-release capability; a later slice must prove that it
   cannot mutate another document before promotion.
10. Explicit delete removes known in-scope DB/FTS rows and reference-aware blobs;
    any failed deletion remains failed and visible.
11. Backup contents are defined by a SQLite snapshot manifest and immutable blob
    hashes.
12. V1 exposes one production implementation per capability. Alternative
    backends remain experimental until they pass the same conformance suite.
13. V1 Vault and backup files are not application-encrypted. Integrity and
    recovery guarantees must never be described as confidentiality.

## Workstreams

| Workstream | Owned surface |
| --- | --- |
| Platform and data kernel | runtime, configuration, storage, jobs, recovery, backup, purge foundations |
| Knowledge pipeline | documents, collections, ingestion, deterministic chunking, SQLite FTS retrieval |
| RAG and product | providers, egress, answers, citations, conversations, API/UI, advanced AI capabilities |
| Integration control | architecture decisions, interface arbitration, commit integration, end-to-end gates, release qualification |

## Delivery discipline

- Deliver vertical user workflows, not isolated repository/service layers.
- Every persisted state transition is atomic, and visible progress never leads
  durable completion.
- Retried core upload and Ask operations do not duplicate work; mutable core
  roots reject stale revisions where concurrent browser edits are possible.
- Tests are offline by default and never use user data or a developer machine's
  database/model service.
- A feature flag cannot conceal an incomplete invariant or preserve duplicate
  production paths indefinitely.
- Java receives no new product features or migration tooling. It remains frozen
  historical reference and is excluded from Go build, runtime, release, and
  user-data flows.

## Accepted decisions

The binding architecture decisions live in [`adr/`](./adr/README.md). A package
or API that conflicts with an accepted ADR must be changed or the ADR must be
superseded explicitly; implementation convenience is not an exception.

End-to-end completion is tracked in
[`acceptance-ledger.md`](./acceptance-ledger.md). A green unit test does not close
a row that requires crash, schema-upgrade, security, or clean-machine evidence.

Gate 0 legacy coverage is tracked in the machine-readable
[`legacy-inventory.csv`](./legacy-inventory.csv), with its human review in
[`legacy-inventory-summary.md`](./legacy-inventory-summary.md). The binding
salvage decisions and their evidence are in
[`legacy-salvage-review.md`](./legacy-salvage-review.md). Run
`docs/rewrite/scripts/validate-legacy-inventory.ps1` whenever a legacy artifact,
feature disposition, salvage decision, data-disposition rule, or acceptance binding
changes.

## Definition of complete

The lean rewrite is complete when every `CORE` capability has a closed
traceability row, core release and disaster-recovery gates pass, Go is the sole
writer of newly created Vaults, `v2/` builds from a clean standalone checkout,
and the extracted product has no Java/MySQL import path or parent-repository
dependency. `LATER` review rows preserve traceability but cannot block this
release.
