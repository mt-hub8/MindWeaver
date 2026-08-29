# BLOB-001 Windows product closure

Status: **PASS**

- Product platform: Windows 10/11 x64
- Executable-evidence commit:
  `768d66edd8ebbfddddac78c485f253f2f221ebb0`
- Recorded: 2026-08-27 (Asia/Shanghai)
- Production/schema/API changes in the closing commit: none

This current-main decision supersedes only the `IMPLEMENTED` status judgment in
the earlier frozen Blob reports. Their measured observations, hashes and stated
limitations remain historical evidence and are not rewritten.

## Closed product invariant

The Windows Blob store now has executable evidence for every state in the
`BLOB-001` product sentence:

- bounded Prepare writes and syncs a private staging file; Open cannot observe
  it before publication;
- publication is content-addressed, one-shot, identity checked and reports a
  typed uncertain outcome after any rename-adjacent ambiguity;
- a real Windows `renamePublished` call refuses an already existing target and
  leaves both source and target regular-file identities and bytes unchanged;
- successful publication to an absent target preserves the opened source file
  identity and exact bytes at the destination, with the source name gone;
- eight Store handles opened on one physical root converge on one object and
  exactly one in-process result reports `Created=true`;
- duplicate, cancellation, durable orphan-candidate, staging cleanup, delete,
  startup sweep and exact-replay paths converge in the frozen K01A/K01B1/K02/
  K03/K04/K06/K09 matrix.

The `Created=true` assertion is deliberately process-local. It is not a
cross-process uniqueness promise; the product's exclusive Vault lock prevents
two product writers, while content identity and verification remain the durable
authority.

## Deleted duplicate blockers

K01B2 (termination inside Sync), K05 (termination inside SQLite's commit
decision) and K08 (unlink before directory Sync returns) expose only the durable
endpoints already covered by the matrix. Adding a user-space hook, journal or
second state machine would manufacture observability without improving the
product state model.

Physical media exhaustion, controller-cache behavior, torn writes and sudden
power loss remain release-machine fault qualifications under `REL-001`; they are
not a second implementation of Blob publication. Linux is not a supported v1
product target. Its current Unix rename helper must be independently replaced
with a fail-closed atomic no-replace implementation and qualified on real Linux
filesystems before any future Linux support claim.

## Verification

From `v2/` with Go 1.27.0 on Windows/amd64:

```powershell
go test ./internal/blob -run '^(TestWindowsRenamePublishedNeverReplacesExisting|TestConcurrentStoreInstancesConvergeOnSameObject)$' -count=100
go test ./internal/blob -count=10
go test ./qualification/knowledge -run '^TestBLOB001' -count=3
go vet ./internal/blob ./qualification/knowledge
```

The first command passed in 7.175 seconds and the complete Blob package passed
ten times in 12.401 seconds. The frozen forced-exit, lifecycle and SQLite Blob
focused gates and both vet packages also passed during independent review.

This PASS is limited to the documented Windows product guarantee. It does not
promote `REL-001`, assert power-loss certification, or add Linux to the supported
platform set.
