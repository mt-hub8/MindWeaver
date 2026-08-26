# BLOB-001 publication and cancellation evidence

Status: **IMPLEMENTED — do not promote BLOB-001 to PASS**

The qualification branch started from
`0dff1d45eee689cb119cd6be20bf19ac40e9d5f6`; the reviewed final slices were
grouped into `d059dc6`, `eaa2b14`, and `e1410b3` on the integration branch.
They change no Blob schema, public route, backup protocol, or OS-specific
rename implementation. The internal Blob error contract is hardened so
callers can mechanically classify publication uncertainty.

## Closed slices

- `eaa2b14` rewrites the real child-process forced-termination qualification.
  A fresh 256-bit nonce and an owner-only, fixed-local sandbox capability
  authorize the child; normal, legacy-environment, and forged-environment
  entries make no Vault write. The child re-reads the committed GC candidate,
  zero-reference state, and exact object bytes before sending one versioned,
  bounded stdout checkpoint bound to the nonce, Blob ID, and size. The parent
  kills and reaps the child, then verifies the orphan through a raw read-only
  file handle before `app.Start`. Startup removes it before route registration.
  Upload, replay, close/reopen, and replay again retain the same document,
  revision, job, and Blob IDs, one object, and zero candidates.
- `d059dc6` makes every full publication hash context-aware. Prepared-source,
  successful-rename, existing-object dedupe, and recovered rename-error
  verification check cancellation before and after every 64 KiB read.
- The same `d059dc6` slice adds `PublicationOutcomeUncertainError`. Its text is stable and
  content-free; `Unwrap` preserves classification, and ID/size/rename outcome
  remain available through both the typed error and non-zero `ImportResult`.
  A real `Store.Import` test uses no database or candidate: post-rename
  cancellation returns `Created=true`, and an exact retry returns
  `Created=false` with the same ID/size and one object. A separate workbench
  test proves its pre-publication GC candidate remains with no DB reference.
- It also extends that contract to both successful-rename and verified
  dedupe cases where publication succeeds but staging cleanup fails: every
  non-success after an object is published or observed retains the result and
  supports `errors.As` to the typed uncertain error.
- `e1410b3` proves the restore owner with the real production `Store.Import`.
  A package-local context fault seam scans at most 17 owner-local entries and
  cancels only after the exact staged Blob object appears. Import itself
  returns the typed post-rename uncertainty. Restore preserves its
  receipt-bound staging tree, does not publish the target Vault, exact startup
  residue recovery removes only that tree, and a fresh clean-machine retry
  reopens the one Blob. No production backup hook remains.
- Existing bounded upload, dedupe, candidate cleanup, shared-blob lifecycle,
  backup/restore/reopen, Windows registered-volume rejection, and non-Windows
  fail-closed backup tests remain supporting evidence.

## Deliberately open acceptance work

Only the publish-to-database-reference point has real-process coverage here.
The superseding checkpoint definitions and their exact open/closed status are
frozen in `blob001-persistence-checkpoint-matrix-b99fb42.md`; that matrix does
not treat the earlier same-process boundary tests as forced-termination proof.

The following also prevent a PASS recommendation:

- Blob-path ENOSPC and short-write qualification at the final-tree boundary;
- certification of Linux no-replace publication behavior (the Linux rename
  path was intentionally not changed by this slice).

The previously open backup object-pin versus purge/sweep interaction is closed
separately by `3cd5f26` and
`docs/rewrite/evidence/blob-backup-interaction.md`; it does not close the
remaining persistence-checkpoint matrix.

## Reproducible commands

Run from `v2/` with the frozen Go 1.27.0 toolchain:

```text
go test ./qualification/knowledge -run '^TestBLOB001PublishedOrphan' -count=10
go test ./internal/blob -run 'Test(PublishCancellation|ImportPreserves|PublicationOutcome|VerifyOpenFile)' -count=10
go test ./internal/workbench -run TestUploadPostRenameCancellation -count=10
go test ./internal/backup -run TestRestoreBlobImportCancellationAfterRename -count=10
go test ./internal/blob ./internal/workbench ./internal/lifecycle ./internal/backup ./qualification/knowledge -count=1
go vet ./internal/blob ./internal/workbench ./internal/lifecycle ./internal/backup ./qualification/knowledge
go test ./... -count=1
go vet ./...
scripts/ci.ps1
scripts/verify-standalone.ps1
```

Passing these commands proves only the closed slices. It does not close the
open matrix or make BLOB-001 release-ready.
