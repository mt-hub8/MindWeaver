# BLOB-001 persistence checkpoint matrix

Status: **FROZEN / PARTIALLY QUALIFIED — BLOB-001 remains IMPLEMENTED**

Baseline: `b99fb422e5a3f119fd91b1b0f5b1fa4c78387b10`.

This matrix freezes the remaining crash boundaries before adding more fault
tests. It does not introduce a journal, change Blob publication, or certify the
Linux no-replace implementation. A checkpoint is closed only by a real child
process that reaches the named production boundary, emits one bounded,
nonce-and-phase-bound and content-free frame, is forcibly terminated, and is
recovered by `app.Start` as the first new Vault owner before routes or workers
are reachable.

## Invariant and fixed recovery oracle

At every boundary, final-tree bytes are either absent or one complete object
whose size and SHA-256 match its canonical Blob ID. Bytes not yet referenced by
the document transaction must remain recoverable as synced staging or through
a committed GC candidate. Startup must remove or account for that state before
ordinary work is reachable. Exact request replay after close/reopen must leave
one document/revision/job/Blob graph, one object and zero candidates.

Before `app.Start`, the parent may inspect only raw read-only filesystem
handles. It must not open the Vault, Blob Store, or SQLite database. Child
authorization uses the existing owner-only fixed-local sandbox plus a fresh
256-bit nonce; stderr is discarded, stdout is strictly bounded and decoded,
and failures expose stable codes rather than paths, source bytes or child
output.

## Frozen checkpoints

| ID | Forced termination boundary | Required post-kill state and recovery | Status at baseline |
|---|---|---|---|
| BLOB-K01 | staging copy is incomplete or the staging file has not completed `Sync` | no final object; stale private staging is removed by the next `app.Start` before routes | OPEN; deterministic write/file-sync fault seam also open |
| BLOB-K02 | `Prepare` returned after staging file and directory sync, before GC candidate commit | exact staging exists; startup removes it; no candidate or final object exists | OPEN |
| BLOB-K03 | GC candidate commit returned, after durable `Prepare`, before publication rename | exact staging and one durable candidate; startup removes staging and resolves the missing-object candidate | CLOSED by `TestBLOB001DurableCandidateBeforeRenameRecoversAfterForcedTermination` |
| BLOB-K04 | publication rename returned and exact final object verifies, before document reference transaction | exact object plus one candidate; startup deletes the unreferenced object and resolves the candidate | CLOSED by `TestBLOB001PublishedOrphanRecoversAfterForcedTermination` |
| BLOB-K05 | document reference transaction is in flight and its commit outcome is not observed | either K04 state, or one referenced graph with no candidate; startup/replay converges without duplicate graph or object | OPEN |
| BLOB-K06 | reference commit returned but upload response is not observed | one referenced graph and no candidate; exact replay before and after reopen returns the committed identities | OPEN for a real kill at this boundary |
| BLOB-K07 | GC has committed/claimed a deletion candidate and is about to unlink the object | candidate and object remain, or the object is already absent; startup retries the same reference-aware decision | OPEN |
| BLOB-K08 | object unlink returned, before its parent directory sync completes | object may be absent but the candidate remains; startup must not report resolution until deletion is durable | OPEN |
| BLOB-K09 | parent directory sync returned, before candidate-resolution commit/response | object is absent and candidate remains or is atomically resolved; startup/replay drains it exactly once | OPEN |

K03 is deliberately separate from the existing same-process lifecycle unit
test. The new qualification child returns from production `Prepare`, commits
and re-reads the candidate, proves zero references and no final object, then
emits the bounded phase/nonce/Blob-ID/size frame and blocks. After kill and
reap, the parent verifies the only staging identity through a raw read-only handle with
exact size and SHA-256. `app.Start` removes that staging file and resolves the
missing-object candidate before route registration. Upload, exact replay,
close/reopen and replay again converge to one graph, one object and zero
candidates. The test passed ten consecutive runs; this closes K03 only.

## Final-tree ENOSPC and short-write seam assessment

The current final tree has deterministic seams for directory sync and rename,
and `writeFull` correctly loops over positive partial writes and rejects a
zero-byte write with `io.ErrShortWrite`. It does **not** have a deterministic
seam at the concrete staging file's `Write` or `Sync` calls. Source-reader
errors and SQLite disk-full tests do not qualify these two Blob boundaries.

The smallest acceptable future seam is package-private and Blob-specific: an
unexported staging-write callback receiving the already-open `*os.File`, plus
an unexported staging-file-sync callback, both installed only through the
package's existing test constructor. It must test partial-write-plus-ENOSPC,
zero-write short write, and file-sync ENOSPC while preserving the real file
identity, Abort cleanup and staging-directory sync. A generic fault framework,
public option, journal, schema change, or weakened file-identity check is out of
scope. Until that seam and a real filesystem qualification are present,
ENOSPC/short-write remains OPEN and cannot support PASS.

## Reproduction

Run from `v2/` with the frozen Go 1.27.0 toolchain:

```text
go test ./qualification/knowledge -run '^TestBLOB001DurableCandidateBeforeRenameRecoversAfterForcedTermination$' -count=10
go vet ./qualification/knowledge
```
