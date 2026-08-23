# Go rewrite architecture decisions

These records are binding for `v2/`. A decision changes only through a new ADR
that names the superseded record and updates the affected acceptance tests.

| ADR | Decision |
| --- | --- |
| [0001](0001-in-repository-rewrite-and-extraction.md) | Build in `v2/`, extract only after qualification |
| [0002](0002-local-runtime-and-vault-ownership.md) | Local single-user runtime and one-writer Vault |
| [0003](0003-authoritative-storage-and-derived-data.md) | SQLite and immutable blobs are authoritative |
| [0004](0004-durable-jobs-events-and-recovery.md) | Original job/event protocol; narrowed by 0012 |
| [0005](0005-provider-egress-and-invocation.md) | Egress rules retained; invocation ledger superseded by 0012 |
| [0006](0006-retrieval-evidence-and-citations.md) | Citation provenance retained; snapshot ledger superseded by 0012 |
| [0007](0007-loopback-api-and-browser-session.md) | Versioned loopback API with bootstrap session |
| [0008](0008-migration-cutover-and-rollback.md) | Verified cutover retained; migration scope narrowed by 0012 |
| [0009](0009-windows-vault-runtime.md) | Handle-identified, exclusively locked Windows Vault runtime |
| [0010](0010-sqlite-driver-qualification.md) | Evidence-gated SQLite driver qualification |
| [0011](0011-salvage-first-vertical-slices.md) | Salvage Java and Go code before proving thin vertical slices |
| [0012](0012-lean-local-core.md) | Replace speculative protocols with observable local vertical slices |
