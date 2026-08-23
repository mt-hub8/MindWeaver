# Go rewrite architecture decisions

These records are binding for `v2/`. A decision changes only through a new ADR
that names the superseded record and updates the affected acceptance tests.

| ADR | Decision |
| --- | --- |
| [0001](0001-in-repository-rewrite-and-extraction.md) | Build in `v2/`, extract only after qualification |
| [0002](0002-local-runtime-and-vault-ownership.md) | Local single-user runtime and one-writer Vault |
| [0003](0003-authoritative-storage-and-derived-data.md) | SQLite and immutable blobs are authoritative |
| [0004](0004-durable-jobs-events-and-recovery.md) | One fenced durable job and event protocol |
| [0005](0005-provider-egress-and-invocation.md) | Default-deny egress and crash-safe invocation |
| [0006](0006-retrieval-evidence-and-citations.md) | One retrieval snapshot and exact answer context |
| [0007](0007-loopback-api-and-browser-session.md) | Versioned loopback API with bootstrap session |
| [0008](0008-migration-cutover-and-rollback.md) | Neutral migration package and verified cutover |
| [0009](0009-windows-vault-runtime.md) | Handle-identified, exclusively locked Windows Vault runtime |
| [0010](0010-sqlite-driver-qualification.md) | Evidence-gated SQLite driver qualification |
