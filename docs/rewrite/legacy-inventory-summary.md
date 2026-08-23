# Legacy inventory summary

This is the Gate 0 inventory of the legacy Java/Python implementation. The
machine-readable source of truth is [`legacy-inventory.csv`](./legacy-inventory.csv);
[`scripts/validate-legacy-inventory.ps1`](./scripts/validate-legacy-inventory.ps1)
rescans the repository and fails if a covered artifact is missing, stale, mapped
to an unknown disposition, or lacks a Go target, migration rule, or acceptance
ledger row.

The inventory records what static source inspection proves. `SOURCE_IDENTIFIED`
means that the artifact/declaration/data field was found in source; it does not
mean that its full runtime or user-visible semantics have been confirmed.
`UNCONFIRMED_GAP` is used for questions that require a live database, deployment
evidence, or real migration fixture.

## Coverage snapshot

The current inventory contains 705 rows:

| Category | Count | Coverage rule |
| --- | ---: | --- |
| Spring Controller endpoints | 119 | Every verb mapping in all 23 `*Controller.java` files, joined with its class route |
| Static UI | 47 | Every shipped file: 22 HTML, 18 JavaScript, and 7 CSS assets; fail-closed stem mapping binds business assets to domain gates |
| Flyway migrations | 33 | Every `V1` through `V33` SQL file |
| Final declared tables | 33 | Every `CREATE TABLE` result |
| Final declared indexes | 101 | Every declared primary, named normal, and named unique index/constraint |
| JPA persistence entities | 33 | Every production class in the legacy `entity` package |
| Scheduler/background components | 22 | Scheduling bootstrap, schedulers, consumers, publishers, runners, executors, handlers, and Rabbit configuration |
| Provider/backend artifacts | 98 | Every source in the LLM/embedding/model-provider/vector/rerank packages, plus provider services/configuration, credential handling, and out-of-package vector implementations |
| Configuration keys | 108 | Property files plus leaf fields from all typed `@ConfigurationProperties` bindings and `@Value` placeholders |
| Environment keys | 10 | Java/Python environment reads, property placeholders, and Compose environment declarations; values are never inventoried |
| External dependencies | 38 | Maven parent/dependencies/plugins/toolchains, Python requirements, Compose images, configured models/services, and probed executables |
| Tracked worker artifacts/endpoints | 20 | 14 files, including tracked bytecode/output samples, plus 6 Python HTTP mappings |
| Legacy scripts/build support | 10 | Windows scripts, Maven wrapper/build files, and Compose files |
| User/operational data categories | 25 | Canonical, sensitive, derived, archival, and ephemeral ownership groups |
| Explicit unresolved gaps | 8 | Items that static repository inspection cannot safely claim as known |

The disposition totals are 32 `KEEP_SEMANTICS`, 482 `REDESIGN`, 36
`REBUILD`, 26 `ARCHIVE_ONLY`, 23 `DEFER`, and 106 `DROP`. These totals count
artifacts, not independent product features.

## Salvage boundary

Legacy disposition records what must happen to an artifact or its data. It no
longer implies feature parity. The binding product decision is the stricter
`salvage_decision`, documented feature by feature in
[`legacy-salvage-review.md`](./legacy-salvage-review.md) and inherited by every
CSV row:

| Salvage decision | Feature count | Artifact count | First-release meaning |
| --- | ---: | ---: | --- |
| `CORE_KEEP_SEMANTICS` | 2 | 20 | Preserve only Collection M:N and Trash/restore user semantics; reuse no Java code |
| `CORE_REBUILD_FROM_ZERO` | 16 | 202 | Need is core, but Java implementation, schema, and protocols are rejected |
| `LATER_FROM_ZERO` | 13 | 351 | Capability, route, schema, job, and UI must be absent from the first release |
| `ARCHIVE_ONLY` | 2 | 26 | Retain checksummed readable history with no executable path |
| `DROP` | 7 | 106 | Do not carry the product or implementation surface into Go |

Thus 222 artifact rows trace to 18 CORE features, but only 20 rows trace to the
two narrow semantics-retention decisions. Artifact volume is not reuse evidence.
Agent/Profile, Memory, Batch, Notification, Evaluation, Qdrant, advanced
Hybrid/RRF/rerank/query-understanding, all embeddings/vector retrieval, reindex,
storage/cache controls, and advanced health/repair are all `LATER_FROM_ZERO`.
Ordinary Task execution,
development mutation endpoints, Java/Spring/Maven, MySQL, RabbitMQ, Python
workers, and the legacy production Compose topology are not Go product scope.

The acceptance ledger now has 41 `CORE` blockers, 8 `LATER` non-blockers, and
2 `ARCHIVE` non-blockers. A later or archive row cannot hold the lean core
release open, and promotion requires a separate accepted vertical slice.

Shared shell/navigation/landing assets remain under `MW-UI-001`. Business
assets are not allowed to collapse into a generic UI row: for example,
`model-settings.*`, `memory-center.*`, `batch-ingestion.*`, `trash.*`, and
`vector-index-health.*` bind to `MW-PRO-001`, `MW-MEM-001`, `MW-BAT-001`,
`MW-TRS-002`, and `MW-HLT-001` respectively, while also carrying `UI-001` and
`UI-002` acceptance IDs. An unknown static stem makes validation fail until an
explicit business mapping is reviewed.

## Decisions added to close disposition holes

The original feature matrix did not explicitly cover several discovered legacy
surfaces. Gate 0 adds these IDs so no inventory row is left without an owner:

| ID | Scope |
| --- | --- |
| `MW-AGT-002` | Agent profiles |
| `MW-NOT-001` | User notifications |
| `MW-CFG-001` | Properties, profiles, and secrets |
| `MW-RUN-001` | Local launch/shutdown/environment tooling |
| `MW-STO-001` | Storage summary and cache controls |
| `MW-CCH-001` | Embedding/retrieval caches |
| `MW-TSK-001` | Ordinary task history, prompts, and results |
| `MW-DEV-001` | Development-only task mutation/dispatch endpoints |
| `MW-RPT-001` | Legacy generated evaluation/benchmark outputs |
| `MW-INF-004` | Java/Spring/Maven runtime and build stack |
| `MW-INF-005` | Optional local Ollama runtime |
| `MW-INF-006` | Legacy Docker Compose development stack |

The validator rejects an empty or unknown `disposition_id`, so the current
machine inventory has zero unowned artifacts. This is traceability closure, not
implementation closure; acceptance rows remain open until their required tests
and reports pass.

## Binding migration rules

- Canonical user inputs and relations are exported read-only, checksummed,
  validated, and imported idempotently. Ambiguous references go to quarantine.
- Chunks, embeddings, vector indexes, and caches are derived data. Their legacy
  rows/volumes can support diagnostics, but Go rebuilds them from verified
  canonical inputs and frozen fingerprints.
- Legacy task, agent, scheduler, outbox, and Rabbit delivery state never enters
  the Go Job state machine. Completed history may be archived; unresolved work
  is reported and requires an explicit user decision.
- Provider secrets are never written to the inventory or neutral export in
  plaintext. Non-secret provider settings translate explicitly; credentials
  require authorized secure rebinding or re-entry.
- PID files, Python environments/bytecode, and broker delivery metadata are not
  migrated. Legacy report files are checksummed and labeled archival, and cannot
  satisfy Go acceptance evidence.
- Qdrant remains optional/deferred and non-authoritative. Its vectors are rebuilt
  only after the backend conformance suite passes.

## Unconfirmed findings that must not be promoted to facts

1. The 101-index count is the migration-declared logical schema. MySQL may create
   or rename supporting foreign-key indexes; supported fixtures and a sanitized
   real migration rehearsal must capture `SHOW CREATE TABLE` and `SHOW INDEX`.
2. Single-file upload code persists extracted `source_text`, while durable source
   bytes were found only in batch staging. Migration must report every document
   lacking original bytes and either import verified text with a degraded-source
   marker or require re-upload.
3. Conversation history is a retained target requirement, but no production
   controller/entity/Flyway implementation was found. Migration must not invent
   conversations from one-shot answers.
4. No application-level HTTP authentication/session enforcement was found in the
   scanned production sources. That does not prove that every historical
   deployment lacked an external control; Go still must pass its own bootstrap,
   session, CSRF, Host, Origin, and CSP attack suite.
5. Source inspection cannot establish real row counts, encodings, orphan rates,
   staging-file availability, or largest-Vault size. `MIG-003` requires a
   sanitized, signed rehearsal report before cutover.
6. Default and Docker profiles name different MySQL databases and ports. The
   exporter must require explicit read-only source selection/fingerprinting and
   may not guess which schema contains the user's data.
7. Encrypted provider keys are recoverable only when the legacy
   `app.security.secret-key` or `MODEL_PROVIDER_SECRET_KEY` is still available.
   Missing keys require credential re-entry; ciphertext/plaintext must not leak
   into reports.
8. `pom.xml` requires Java 21, while the Windows check script says JDK 17+.
   Java 17 compatibility is unproven; the conflict is archived and must not be
   copied into Go release prerequisites.

## Offline validation

From the repository root:

```powershell
& .\docs\rewrite\scripts\validate-legacy-inventory.ps1
```

`-ListDiscovered` prints the current mapping, and `-EmitInventory` emits a
candidate CSV to standard output for review. Emission never overwrites the
committed inventory; reviewed edits must still be applied deliberately. The
validator also requires every feature to have exactly one ten-field salvage
review, enforces the salvage/scope pairing in all 705 rows, and verifies that
only `CORE` acceptance rows have `Core gate=YES`.
