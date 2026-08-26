# Legacy inventory summary

This is the Gate 0 inventory of the legacy Java/Python implementation. The
machine-readable source of truth is [`legacy-inventory.csv`](./legacy-inventory.csv);
[`scripts/validate-legacy-inventory.ps1`](./scripts/validate-legacy-inventory.ps1)
rescans the repository and fails if a covered artifact is missing, stale, mapped
to an unknown disposition, or lacks a Go target, data-disposition rule, or acceptance
ledger row.

The inventory records what static source inspection proves. `SOURCE_IDENTIFIED`
means that the artifact/declaration/data field was found in source; it does not
mean that its full runtime or user-visible semantics have been confirmed.
`UNCONFIRMED_GAP` is used for questions that would require a live legacy
database or deployment evidence. Those unknowns are recorded as reasons not to
read or convert legacy data; they are not release work.

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

The disposition totals are 31 `KEEP_SEMANTICS`, 402 `REDESIGN`, 80
`REBUILD`, 23 `DEFER`, and 169 `DROP`. These totals count
artifacts, not independent product features.

## Salvage boundary

Legacy disposition records what must happen to an artifact or its data. It no
longer implies feature parity. The binding product decision is the stricter
`salvage_decision`, documented feature by feature in
[`legacy-salvage-review.md`](./legacy-salvage-review.md) and inherited by every
CSV row:

| Salvage decision | Feature count | Artifact count | First-release meaning |
| --- | ---: | ---: | --- |
| `CORE_REQUIREMENT_ONLY` | 2 | 20 | Preserve only Collection M:N and Trash/restore requirements; reuse no Java code, schema, identifier, or stored record |
| `CORE_REBUILD_FROM_ZERO` | 15 | 165 | Need is core, but Java implementation, schema, protocols, and stored state are rejected |
| `LATER_FROM_ZERO` | 13 | 351 | Capability, route, schema, job, and UI must be absent from the first release |
| `DROP` | 10 | 169 | Do not carry the product, implementation, or legacy data surface into Go |

Thus 185 artifact rows trace to 17 CORE features, but only 20 rows trace to the
two narrow requirements-retention decisions. Artifact volume is not reuse evidence.
Agent/Profile, Memory, Batch, Notification, Evaluation, Qdrant, advanced
Hybrid/RRF/rerank/query-understanding, all embeddings/vector retrieval, reindex,
storage/cache controls, and advanced health/repair are all `LATER_FROM_ZERO`.
Ordinary Task execution,
development mutation endpoints, Java/Spring/Maven, MySQL, RabbitMQ, Python
workers, and the legacy production Compose topology are not Go product scope.

The acceptance ledger now has 38 `CORE` blockers and 8 `LATER` non-blockers.
A later row cannot hold the lean core release open, and promotion requires a
separate accepted vertical slice. Retired `MIG-*` and `HIS-*` IDs are rejected
by the validator and cannot re-enter the inventory.

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
| `MW-TSK-001` | Dropped ordinary task history, prompts, and results |
| `MW-DEV-001` | Development-only task mutation/dispatch endpoints |
| `MW-RPT-001` | Dropped legacy generated evaluation/benchmark outputs |
| `MW-INF-004` | Java/Spring/Maven runtime and build stack |
| `MW-INF-005` | Optional local Ollama runtime |
| `MW-INF-006` | Legacy Docker Compose development stack |

The validator rejects an empty or unknown `disposition_id`, so the current
machine inventory has zero unowned artifacts. This is traceability closure, not
implementation closure; acceptance rows remain open until their required tests
and reports pass.

## Binding historical data dispositions

- The Go product does not read, export, translate, quarantine, archive into the
  product, or import any Java/MySQL user or operational state.
- Users create a fresh Vault and add supported TXT, Markdown, and text PDF files
  through the ordinary bounded Go upload path. Only those new Go-owned bytes and
  records can become canonical product state.
- Legacy chunks, embeddings, vector indexes, caches, task/agent/scheduler state,
  reports, worker outputs, broker messages, and container volumes remain outside
  the Go product and release evidence.
- Legacy provider settings, ciphertext, credentials, environment values, and
  process metadata are not read or translated. Optional loopback Ollama is
  configured and probed afresh through the Go product.
- Java source and schemas remain only historical review evidence in this parent
  repository. They are absent from the extracted repository and the exact two-
  executable Windows release.
- Normal SQLite `001` through `006` migrations remain Go-to-Go schema evolution;
  they do not authorize a legacy-data adapter or compatibility path.

## Unconfirmed findings that must not be promoted to facts

1. The 101-index count is the Flyway-declared logical schema. MySQL may create or
   rename supporting foreign-key indexes; no product claim is made about a live
   legacy database and no live-schema probe ships.
2. Single-file upload code persists extracted `source_text`, while durable source
   bytes were found only in batch staging. This is one reason database conversion
   is unsupported; users re-upload available supported source files.
3. Conversation history is a retained target requirement, but no production
   controller/entity/Flyway implementation was found. Go creates conversation
   state only from new product interactions.
4. No application-level HTTP authentication/session enforcement was found in the
   scanned production sources. That does not prove that every historical
   deployment lacked an external control; Go still must pass its own bootstrap,
   session, CSRF, Host, Origin, and CSP attack suite.
5. Source inspection cannot establish real row counts, encodings, orphan rates,
   staging-file availability, or largest-Vault size. No conversion or capacity
   promise is inferred from the repository.
6. Default and Docker profiles name different MySQL databases and ports. The
   Go product must not guess, probe, or open either legacy schema.
7. Encrypted provider keys and their legacy master keys are outside the Go
   product. No exporter exists and neither ciphertext nor plaintext is read.
8. `pom.xml` requires Java 21, while the Windows check script says JDK 17+.
   Java 17 compatibility is unproven; the conflict is archived and must not be
   copied into Go release prerequisites.

## Offline validation

From the repository root:

```powershell
& .\docs\rewrite\scripts\validate-legacy-inventory.ps1
```

`-ListDiscovered` prints the current mapping, and `-EmitInventory` emits a
candidate CSV to standard output for review. The validator first validates the
feature matrix, salvage review, and acceptance ledger even in emission mode. It
requires every feature to have exactly one ten-field salvage review, enforces
the salvage/scope pairing and exact twelve-column schema in all 705 rows, rejects
retired migration/history acceptance IDs, and verifies that only `CORE`
acceptance rows have `Core gate=YES`.
