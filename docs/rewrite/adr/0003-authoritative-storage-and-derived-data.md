# ADR 0003: Authoritative storage and derived data

- Status: Accepted
- Date: 2026-08-23

## Decision

SQLite is authoritative for identities, lifecycle, membership, versions,
operations, lineage, and active-generation pointers. Content-addressed,
immutable blobs are authoritative for imported source bytes and durable external
results. Chunks, FTS, embeddings, vector indexes, previews, and caches are
derived and rebuildable.

Files use an intent protocol: record intent, write and sync staging, verify hash,
atomically rename, then commit the database reference. Database migrations are
short, versioned, transactional where SQLite permits, and never perform model
calls or large derived-data rebuilds.

## Consequences and verification

- Retrieval uses only an active, validated generation of an active document.
- Blob reference counts are never the sole deletion proof; reachability, pins,
  backup barriers, and lineage are rechecked before garbage collection.
- Backups enumerate blobs from their own SQLite snapshot and verify every hash.
- Restore builds and verifies a new Vault before any switch; it does not merge
  into the currently open Vault.
