# Legacy Java domain and backend completion

## Decision

For rewrite planning, "complete" means that every legacy Java domain rule or
backend capability judged valuable for the first Go product has an owned Go
implementation and executable acceptance evidence. It does **not** mean a
line-for-line Java translation, restoration of the old runtime, or completion
of installer and release qualification.

That scope is complete in the repository tree containing this record.

The final code closure is
`8a9195d5a20ddef3acfeacc974a049724fc2d34f` (`fix(v2): interrupt active
ingestion cancellation`). Its end-to-end test drives HTTP cancellation through
the durable SQLite request, the exact active worker and workbench operation,
the PDF helper process, terminal document/job state, a subsequent successful
job, and Vault close/reopen. The shipped command set remains exactly
`mindweaver.exe` and `mindweaver-pdf.exe`.

The binding disposition contains exactly 40 legacy capability rows: 17 `CORE`,
13 `LATER`, and 10 `DROP`. The 17 CORE requirements have Go-owned product
boundaries. LATER capabilities are deliberately absent until a new vertical
slice is accepted, and DROP capabilities must not be restored from Java.

## Closed Go-owned capability groups

- Bounded TXT, Markdown, and text-PDF upload; immutable source blobs;
  deterministic chunking; durable ingestion; idempotent replay; and crash
  recovery.
- Document lifecycle, trash, restore, permanent purge, FTS projection cleanup,
  reference-aware blob cleanup, and true many-to-many collections.
- Scoped literal continuous-phrase SQLite FTS for Chinese and English source
  text, deterministic ordering, explicit empty scope, and lifecycle isolation.
- Keyword/phrase-driven Ask, durable conversations and answers, source-bound
  structural citations, honest no-context refusal, and bounded failure or
  outcome-uncertain states.
- Optional literal-loopback Ollama with versioned non-secret configuration,
  no ambient proxy or redirect authority change, bounded invocation, and no
  unsafe automatic provider replay.
- A single locked Vault, fresh Go-owned SQLite schema upgrades, one fenced
  durable job protocol, bounded shutdown, startup reconciliation, and
  standalone backup verification and restore to a new Vault.
- Versioned fail-closed configuration, bounded HTTP/OpenAPI contracts, and the
  embedded local product surface required to exercise the backend workflows.
- Durable cancellation of a running ingestion: cancellation authority is
  written before interruption, only the matching active job is interrupted,
  helper descendants are reaped, the job and document become `cancelled`, the
  worker continues with the next job, and reopen preserves both outcomes.

The detailed capability-to-evidence mapping remains authoritative in
[`feature-disposition.md`](./feature-disposition.md) and
[`acceptance-ledger.md`](./acceptance-ledger.md).

## Why no Java implementation is retained

The exact legacy review covers 890 unique runtime/build files and 75,516 lines.
Every legacy implementation file is `DROP`; there are zero source-code KEEP
exceptions. The retained value is the reviewed requirement or invariant, now
owned by Go, not a Java class, schema, identifier, stored record, or runtime
topology. See
[`evidence/legacy-product-review/review-summary.md`](./evidence/legacy-product-review/review-summary.md)
and
[`evidence/legacy-platform-review/review-04-summary.md`](./evidence/legacy-platform-review/review-04-summary.md).

The old Java/Spring/MySQL/Rabbit/Python tree remains historical evidence only.
The Go product creates a fresh Vault and exposes no legacy exporter, importer,
migration command, third executable, or Java/MySQL data-upgrade path.

The historical source itself was not deleted during this closure. Relative to
the immutable review baseline
`0df22ddaf02c64bf73a7df12cd5fea6b52632c73`, the current tree has no path or
content change under `src/`, `.mvn/`, `mvnw*`, `pom.xml`, `workers/`, or the
legacy Compose files. `src/` still contains 866 tracked files, including 780
Java files, exactly matching that baseline.

For an additional stable recovery name, annotated tag
`archive/legacy-java-v19-20260829` points to the historical Java main commit
`65f6622ea8dc65ab7f5c155c5b58e8c2db349637`. The tag is archival evidence; it
is not a Go build input or a supported legacy runtime.

## Explicitly outside this completion statement

- Natural-question understanding, embeddings/vector retrieval, Hybrid/RRF,
  reranking, reindex, Agent, Profile, Memory, Batch, Notification, Evaluation,
  Qdrant, cache/storage consoles, and advanced repair remain LATER or DROP.
- Approved real-browser security/accessibility execution remains tracked by
  `SEC-001`, `UI-001`, and `UI-002`.
- Race/physical-fault/long-soak qualification and signed MSI, project license,
  final SBOM/NOTICE, upgrade/uninstall, and clean-machine evidence remain under
  `REL-001` and `REL-002`.

Those rows describe qualification or future product scope. They do not reopen
the completed Java-domain/backend salvage decision.

## Preservation rule

Committed legacy material remains recoverable through Git history. Before any
large uncommitted WIP is deleted, reverted, or reset, preserve an external
working-file snapshot, staged and unstaged patches, untracked files,
branch/HEAD/status metadata, and a verified SHA-256 manifest. A failed backup
verification forbids the deletion.

The large historical WIP cleanups associated with this closure have verified
external backup receipts. The archives themselves remain outside Git so they
cannot become product inputs:

| Logical archive | Files | SHA-256 |
| --- | ---: | --- |
| `rag-provider-api-migration-f530291.zip` | 39 | `4a3aed8c01baa955e39deed88c6b6c67a636e5a1152c0075473c210e4460d92b` |
| `knowledge-ingestion-retrieval-049ebb4.zip` | 7 | `0315fa9f2b386ed9d0f62a42c40d1e62e788b8fd21cc07820786825e8769464e` |
| `knowledge-core-qualification-cdd0f40.zip` | 7 | `e56ed6abdb6c7435d708b289bf9afb02bab379d39d3a21ed96f95b403287f47d` |
| `legacy-import-7d831cf-1f1a9dc.zip` | 4 | `41ab7c4c40076edec17adfd36a81ab0e0a134399e3812d38ccc1c94a00d81c46` |
| `2026-08-29-v2-final-closure-preflight-complete.zip` | 2 untracked + tracked patches | `b5311b98e06274284ee337bf022c933312a4e1c4daa923c71247a5f3f2000972` |
| `v2-vendor-head.zip` | 698 tracked dependency files | `feb9e6ed816d5165fbea348c9b89d0aedb59c6b633e449e3b52fcd2ff5d36cbe` |

Each receipt represents the working files, staged and unstaged patches,
untracked files, Git metadata, and a per-file hash manifest. These backups are
preservation records, not accepted Go implementation or release evidence.
The final closure archive contains seven manifest entries, including the
saved status, staged and unstaged patches, copied untracked files, metadata,
and the per-file SHA-256 manifest. The earlier archive with the same date but
without the `-complete` suffix failed completeness verification and is not a
valid receipt.

The supply-chain qualification fixtures are regenerated against the final
code closure above so ordinary repository gates test the current two PE
inputs. Their existing project-license, release-metadata, vulnerability, and
final-source-attestation blockers remain intact; this regeneration is not a
release qualification or SBOM approval.
