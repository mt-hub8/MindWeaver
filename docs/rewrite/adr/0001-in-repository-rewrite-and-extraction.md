# ADR 0001: In-repository rewrite and extraction

- Status: Accepted; legacy-migration clauses superseded by ADR 0014
- Date: 2026-08-23

## Decision

Build the replacement as the independent Go module `v2/` with module path
`github.com/mt-hub8/MindWeaver/v2`. The Java application remains a read-only
source of historical product evidence. It is not a supported data source or an
implementation template. No Go production package may import, execute, or
discover files outside `v2/`.

Extract `v2/` to a new repository only after standalone build, packaging,
disaster recovery, release, and fresh-Vault absence gates pass. Until then,
integration happens on `codex/go-rewrite` in this repository.

## Consequences and verification

- Java remains frozen historical evidence and receives no product, export, or
  migration changes.
- `scripts/verify-standalone.*` copies `v2/` to an empty directory and must pass
  format, test, vet, and build with network dependency resolution disabled.
- Extraction is a release operation, not a second implementation project.
