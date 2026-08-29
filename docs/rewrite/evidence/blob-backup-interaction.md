# BLOB-001 backup/deletion interaction qualification

Status: **IMPLEMENTED; not release-qualified**

Baseline: `codex/go-rewrite@0dff1d45eee689cb119cd6be20bf19ac40e9d5f6`.
The executable evidence is
`TestCreatePinExcludesPurgeAndSweepThroughCleanRestore` in the Windows backup
test package. This slice changes no production code or schema.

With the frozen Go 1.27.0 toolchain and network lookup disabled, the focused
qualification passed ten consecutive real executions:

```text
go test ./internal/backup -run ^TestCreatePinExcludesPurgeAndSweepThroughCleanRestore$ -count=10
ok github.com/mt-hub8/MindWeaver/v2/internal/backup
```

The test uses a real temporary Vault boundary, SQLite Store, Blob Store,
Workbench ingestion, lifecycle Service, backup Coordinator, and clean-machine
recovery. The only test control is the Coordinator's existing internal
publication hook after the SQLite snapshot has passed integrity and canonical
qualification but before its commitment and referenced-blob copy. The public
`Create` method calls the same `create` implementation with empty hooks.

At that checkpoint the test proves the Coordinator already owns the shared
object pin: real purge and sweep calls enter `BeginDeletionContext` and invoke
the waiting context's `Done` method, but neither returns or mutates rows while
the pin remains held. A canceled purge and a deadline-expired sweep return
their stable context errors. A new shared pin can then be acquired while the
backup pin remains live, proving the canceled writers did not leak or strand
the barrier.

After release, Create copies and verifies the exact snapshot-referenced blob
and publishes one backup. The queued purge then removes the live document,
revision, job, FTS rows, and source blob; the queued sweep completes without a
duplicate candidate. The published backup is verified again after that source
deletion. A one-shot clean-machine restore is reopened through the real Vault,
SQLite, Blob, and Workbench boundaries and must retain:

- one committed blob with exact size, SHA-256, and bytes;
- the pre-trash document and revision identity;
- the pre-trash chunk/document/revision search identity;
- SQLite integrity and canonical consistency;
- an unchanged backup tree after restore.

Runtime paths, source text, and durable identifiers are never written to this
evidence document. BLOB-001 remains `IMPLEMENTED`; this interaction test does
not close soak, release packaging, or clean-VM qualification.
