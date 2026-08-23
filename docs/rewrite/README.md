# MindWeaver Go rewrite execution charter

## Objective

Replace the Java implementation with a local-first Go application that can be
installed, operated, backed up, restored, migrated, and removed without MySQL,
RabbitMQ, or Python workers. The Java tree is a read-only source of product
knowledge and legacy data, not an implementation template.

The Go implementation is developed under `v2/` and must be independently
buildable from its first executable milestone. It will be extracted to a new
repository only after migration and release qualification pass.

## Product boundary

- Single user and one active Vault per process.
- Windows 10/11 x64 is the first Tier-1 platform.
- Offline-first; Ollama is the default optional model runtime.
- Cloud providers require explicit configuration and egress consent.
- Text, Markdown, and text-based PDF are the initial document formats.
- No account, collaboration, cloud synchronization, or implicit LAN mode.

## Architectural invariants

1. SQLite is authoritative for state, membership, lifecycle, and the active
   document generation.
2. Immutable blobs are authoritative for imported source bytes.
3. FTS, chunks, embeddings, vector indexes, and caches are rebuildable derived
   data.
4. Every asynchronous operation uses one durable job protocol with atomic claim,
   fencing, bounded retry, cancellation, and recovery.
5. External I/O never runs inside a database transaction.
6. Ask, Agent, and Evaluation use one retrieval engine and one immutable
   retrieval snapshot model.
7. A citation can reference only an AnswerContext item actually sent to the
   model.
8. Strict refusal changes the returned answer; it is not diagnostic metadata.
9. Reindex atomically switches one document and cannot mutate another document.
10. Purge cannot succeed until all planned in-Vault copies are verified removed
    or irreversibly redacted.
11. Backup contents are defined by a SQLite snapshot manifest and immutable blob
    hashes.
12. V1 exposes one production implementation per capability. Alternative
    backends remain experimental until they pass the same conformance suite.

## Workstreams

| Workstream | Owned surface |
| --- | --- |
| Platform and data kernel | runtime, configuration, storage, jobs, recovery, backup, purge foundations |
| Knowledge pipeline | documents, collections, ingestion, chunking, generations, lexical/vector retrieval |
| RAG and product | providers, egress, answers, citations, conversations, API/UI, migration, advanced AI capabilities |
| Integration control | architecture decisions, interface arbitration, commit integration, end-to-end gates, release qualification |

## Delivery discipline

- Deliver vertical user workflows, not isolated repository/service layers.
- Every persisted state transition and its event commit in the same transaction.
- Every write endpoint is idempotent and every mutable root is versioned.
- Tests are offline by default and never use user data or a developer machine's
  database/model service.
- A feature flag cannot conceal an incomplete invariant or preserve duplicate
  production paths indefinitely.
- Java receives no new product features; only export, migration, or critical data
  safety fixes are allowed.

## Definition of complete

The rewrite is complete only when every retained legacy capability has a closed
traceability row, all canonical data migrates with a machine-readable verification
report, release and disaster-recovery gates pass, Go is the sole writer, `v2/`
builds from a clean standalone checkout, and the Java repository is archived with
a supported migration exit.
