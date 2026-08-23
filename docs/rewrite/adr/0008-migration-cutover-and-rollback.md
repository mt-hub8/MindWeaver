# ADR 0008: Migration, cutover, and rollback

- Status: Accepted
- Date: 2026-08-23

## Decision

Migration crosses a neutral, versioned package produced by a read-only Java
exporter and consumed idempotently by Go. Canonical source files, document
lifecycle, collection membership, conversations, explicit memory, and supported
configuration are migrated. Chunks, embeddings, vector indexes, caches, job
leases, and ambiguous execution state are rebuilt or quarantined rather than
copied as truth.

Import writes into a new Vault, records source identity and per-record outcome,
and emits a machine-readable verification report. Cutover occurs only after
hash/count/relation verification, backup/restore rehearsal, and user-visible
exception review. The old Java data stays read-only through the rollback window.

## Consequences and verification

- Running the same import twice creates no duplicate canonical object.
- Unsupported or inconsistent legacy rows enter quarantine with stable reasons;
  they are never silently dropped or guessed.
- Rollback means reopening the untouched legacy system read-only or restoring a
  pre-cutover backup; Go never writes legacy stores.
- Go becomes the sole writer only after Gate 8 passes, and repository extraction
  follows the final verified migration package.
