# ADR 0011: Salvage first and prove vertical slices

- Status: Accepted
- Date: 2026-08-23

## Context

The Java repository contains many partially connected features, conflicting
contracts, and infrastructure choices whose existence does not prove product
value. The first Go work repeated part of that risk: several thousand lines of
pure protocols and state validators were created before a document could be
uploaded, persisted, retrieved, and answered through a real local HTTP flow.
Passing unit tests for a self-defined protocol is not evidence that the protocol
is needed or that the product works.

The rewrite must not preserve either Java compatibility or already-written Go
code merely because work has been invested in it.

## Decision

Every legacy capability and every Go package receives an explicit salvage
decision. The allowed outcomes are:

- `CORE_REQUIREMENT_ONLY`: preserve a proven narrow user-visible requirement,
  but no legacy code, schema, identifier, or stored record.
- `CORE_REBUILD_FROM_ZERO`: the need belongs in the first local-workbench
  release, while the existing Java/Go implementation is discarded.
- `SIMPLIFY_NOW`: retain only a named invariant or small implementation fragment
  and replace the surrounding abstraction with the smallest working design.
- `LATER_FROM_ZERO`: exclude it from the first release and design it later from
  current product evidence, not from legacy compatibility.
- `DROP`: neither code nor behavior enters the Go product.

Code, documentation, tests, and endpoint count are not value evidence by
themselves. A first-release item must identify a real user job, canonical data
ownership, a complete success/failure/recovery path, and an executable
acceptance test. Unknown or weakly supported items default to
`LATER_FROM_ZERO` or `DROP`.

Implementation proceeds in thin vertical slices. A slice is not complete until
one real entry point crosses its actual security, database, blob, and domain
boundaries and is verified through the public interface. New general-purpose
protocols, registries, event schemas, state machines, or compatibility fields
require at least two concrete consumers and a demonstrated invariant that a
smaller local implementation cannot enforce.

The initial Go core is limited to:

1. one locally owned Vault with SQLite, immutable source blobs, recovery, and
   backup;
2. documents, lifecycle, true many-to-many collections, deterministic
   ingestion, and one simple trustworthy retrieval path;
3. provider configuration and safe transport, a minimal crash-aware call
   journal, answer context, citations/refusal, and conversations;
4. a loopback-only authenticated API and embedded UI;
5. new-Vault backup/restore and versioned Go schema upgrades.

Agent workflows, agent profiles, memory, batch ingestion, notifications,
evaluation, Qdrant, advanced hybrid/rerank/query-understanding experiments, and
advanced repair consoles are `LATER_FROM_ZERO` unless a later ADR promotes a
specific, evidence-backed slice.

## Consequences for existing Go code

The current `v2/` tree is candidate material, not a protected baseline.
Protocol-only packages may be deleted even when their tests pass. In particular:

- Job, ingestion, generation, and provider-call state must not model the same
  execution facts in multiple aggregates.
- Collection membership belongs in normalized storage; an in-memory aggregate
  containing the complete catalog is not the production model.
- Event records are constructed by the transaction that owns the state change;
  callers do not supply a generic envelope whose payload can disagree.
- Provider capabilities and invocation states include only behavior exercised
  by a real adapter in the current release.
- OpenAPI describes running handlers; speculative endpoints and schemas are
  removed.

The isolated SQLite spike remains evidence rather than production code. A
passing spike can qualify a dependency candidate but cannot close the product
storage gate.

## Gate

Before implementation resumes, the repository must contain:

1. a complete Java feature/data salvage matrix;
2. a package-by-package Go keep/simplify/defer/delete review;
3. a first-release surface small enough to exercise end to end;
4. acceptance rows explicitly marked `CORE` or `LATER`.

No candidate branch is merged solely because it is internally consistent or
test-green.
