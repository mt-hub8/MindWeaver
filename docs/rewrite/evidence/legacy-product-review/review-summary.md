# Legacy product review closure

## Coverage ledger

The immutable legacy baseline is
`0df22ddaf02c64bf73a7df12cd5fea6b52632c73`. Detailed manifests intentionally
contain comparison and dependency evidence, so their partition-local totals
must not be added. Repository-wide coverage comes from two disjoint masters:

| Unique master | Files | Lines |
| --- | ---: | ---: |
| Delegated Java/test/evaluation/static responsibility set | 223 | 24,065 |
| Remaining legacy runtime/build product surface | 667 | 51,451 |
| **Complete legacy runtime/build union** | **890** | **75,516** |

The primary master includes all required nine main/test Java package trees,
both evaluation resources, and all 47 static files. Its 176 non-static files
occur exactly once across the RAG/Provider, Agent/Memory, and
Evaluation/Batch/KBHealth detailed slices. The static master is the sole
repository-wide owner of its 47 files / 10,223 lines; earlier references to
eight static files do not count again.

Every one of the 890 legacy files is `DROP`. There are zero implementation
exceptions. The Java/MySQL exporter, neutral package, importer,
`mindweaver-migrate`, and SQLite 007 history is not part of these manifests and
remains dropped by architecture decision.

## Findings and disposition

The legacy surfaces contain actionable P0 evidence when considered as runnable
software, including unauthenticated externally bindable Python model workers,
stored-data-to-`innerHTML` paths, and privileged browser mutations without an
authentication/CSRF/idempotency/revision protocol. Partition reports also
record P1 failures in grounding assurance, cancellation, transaction scope,
resource bounds, provider/worker error handling, batch atomicity, health and
cleanup claims, and real-browser qualification. These findings reinforce
`DROP`; they are not a repair backlog for Java.

The current Go comparison set has no new P0 in this review. Its structural
citation boundary is now stated honestly and protected by a negative UI test.
The remaining P1 qualification gap is a real browser release gate: no pinned
offline browser artifact or independent Windows executable workflow exists, so
UI security/accessibility evidence remains `NOT_IMPLEMENTED` / `BLOCKED`.

Go RAG, literal-loopback Ollama, durable conversation/answer state, embedded
WebUI, and OpenAPI remain `KEEP/HARDEN`. Agent, Memory, Evaluation, Batch,
KBHealth, Notification, embeddings/vector, rerank, reindex, and related
experiments remain absent from the production contract. Normal Go schema
migrations, TXT/Markdown/PDF ingestion, Backup/Restore, and Go-to-Go upgrades
are explicitly outside the legacy-drop decision.

## Reproducible checks

`verify.ps1` reads each immutable Git blob rather than the working tree,
recomputes SHA-256 and line coverage, and adds manifest-specific exact-set,
ownership, decision, complement, and union checks. Offline Go 1.27 package
tests and vet cover the current Go comparison boundary. No Java service,
Python worker, browser, model, network provider, MySQL source, or user data is
started or consumed by this review.
