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
| BLOB-K01A | staging copy has written an exact prefix but has not observed source EOF, so file `Sync` has not started | incomplete private staging and no final object; startup removes staging before routes | CLOSED by `TestBLOB001IncompleteStagingRecoversAfterForcedTermination` |
| BLOB-K01B | staging `Write` or file/directory `Sync` fails or its outcome is not observed | no partial final object; private staging is removed or explicitly recoverable | OPEN; deterministic write/file-sync fault seam remains absent |
| BLOB-K02 | `Prepare` returned after staging file and directory sync, before GC candidate commit | exact staging exists; startup removes it; no candidate or final object exists | CLOSED by `TestBLOB001DurableStagingBeforeCandidateRecoversAfterForcedTermination` |
| BLOB-K03 | GC candidate commit returned, after durable `Prepare`, before publication rename | exact staging and one durable candidate; startup removes staging and resolves the missing-object candidate | CLOSED by `TestBLOB001DurableCandidateBeforeRenameRecoversAfterForcedTermination` |
| BLOB-K04 | publication rename returned and exact final object verifies, before document reference transaction | exact object plus one candidate; startup deletes the unreferenced object and resolves the candidate | CLOSED by `TestBLOB001PublishedOrphanRecoversAfterForcedTermination` |
| BLOB-K05 | document reference transaction is in flight and its commit outcome is not observed | either K04 state, or one referenced graph with no candidate; startup/replay converges without duplicate graph or object | OPEN |
| BLOB-K06 | reference commit returned but upload response is not observed | one referenced graph and no candidate; exact replay before and after reopen returns the committed identities | CLOSED by `TestBLOB001ReferenceCommitResponseReplayAfterForcedTermination` |
| BLOB-K07 | GC has committed/claimed a deletion candidate and is about to unlink the object | candidate and object remain, or the object is already absent; startup retries the same reference-aware decision | OPEN |
| BLOB-K08 | object unlink returned, before its parent directory sync completes | object may be absent but the candidate remains; startup must not report resolution until deletion is durable | OPEN |
| BLOB-K09 | parent directory sync returned, before candidate-resolution commit/response | object is absent and candidate remains or is atomically resolved; startup/replay drains it exactly once | OPEN |

K01A, K02 and K03 share only a qualification harness, not production state.
For K01A a bounded reader returns an exact source prefix, then emits a frame and
blocks on the next `Read`. Production `copyBounded` calls that second `Read`
only after writing the prefix; because EOF has not been observed, `Prepare` has
not reached file `Sync`. The frame is explicitly `complete=false` and binds the
phase, nonce, prefix content address and prefix size rather than claiming a
final Blob ID. K02 and K03 return from production `Prepare`, prove zero
references and no final object, and emit `complete=true` frames. K02 proves
there is no candidate; K03 commits and re-reads exactly one candidate.

After kill and reap, the parent verifies the only staging identity through a
raw read-only handle with exact size and SHA-256. `app.Start` removes that
staging file before route registration; K03 additionally resolves exactly one
missing-object candidate, while K01A and K02 process none. Upload, exact replay,
close/reopen and replay again converge to one graph, one object and zero
candidates. Each test must pass ten consecutive runs before its row is closed.
These tests are deliberately separate from the same-process lifecycle unit
tests, which cannot prove forced termination or first-owner ordering. K01A does
not close K01B or qualify ENOSPC/short-write behavior.

K06 runs the real workbench upload through Blob publication and the atomic
document-reference transaction. After `Upload` returns, the child re-reads one
document/source/job graph, one referenced Blob and zero candidates, then emits
a `complete=true` frame bound to the phase, nonce, Blob content, and three
distinct canonical document/revision/job IDs. The parent kills before it can
observe an upload response, verifies the final object only through a raw
read-only handle, and lets `app.Start` become the first new Vault owner. Before
route registration startup observes the exact queued graph and must clean or
sweep nothing. Replaying the same key/body before and after another reopen
returns `Created=false` with all four original IDs and leaves one graph/object.
This closes committed-response loss only; it does not close the in-flight
transaction outcome in K05.

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
go test ./qualification/knowledge -run '^TestBLOB001(IncompleteStaging|DurableStagingBeforeCandidate|DurableCandidateBeforeRename)RecoversAfterForcedTermination$' -count=10
go test ./qualification/knowledge -run '^TestBLOB001ReferenceCommitResponseReplayAfterForcedTermination$' -count=10
go vet ./qualification/knowledge
```
