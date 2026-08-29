# ADR 0014: Fresh-Vault adoption and no Java/MySQL data migration

- Status: Accepted
- Date: 2026-08-26
- Supersedes: the legacy-migration portions of ADRs 0001, 0011, and 0012, and
  all of ADR 0008

## Context

There is no required legacy Java/MySQL user dataset to preserve. Building and
qualifying a Java exporter, a neutral interchange package, an idempotent Go
importer, a migration CLI, and a dedicated SQLite receipt schema would add a
large security and recovery surface without serving the target personal
workbench. Existing candidate code does not gain product value merely because
it is internally rigorous or already written.

## Decision

The Go product starts from a newly created Vault. Users install the Go release,
create that Vault, upload TXT, Markdown, or supported text PDF sources, and let
the Go ingestion pipeline build chunks and FTS indexes.

The release contains no Java exporter, neutral legacy package, legacy importer,
`mindweaver-migrate` command, legacy-import SQLite migration, or old-data
quarantine/report workflow. Candidate branches containing those components are
not merged. Java, Spring, Maven, Flyway, and MySQL sources may remain in this
repository as read-only historical evidence while the rewrite is in place, but
they are excluded from Go compilation, runtime, packaging, release evidence,
and user-data flows.

This decision does not remove ordinary Go database evolution. The application
must still migrate its own supported SQLite schema versions during a Go
upgrade, and those migrations remain covered by compatibility, rollback,
backup, corruption, and clean-install tests. New-Vault backup and restore also
remain CORE.

## Consequences and verification

- Remove `MIG-001` through `MIG-003` and `HIS-001` through `HIS-002` from the
  acceptance ledger.
- Mark legacy data, task history, and legacy report import surfaces `DROP`.
- `CUT-001` proves a clean first run and absence of Java/MySQL import surfaces,
  not a legacy-data cutover.
- Final release artifacts contain exactly the retained Go application and PDF
  helper executables; no migration executable or Java runtime is packaged.
- Dependency, command-surface, standalone, and package-content gates fail if a
  legacy exporter/importer or Java/MySQL runtime dependency reappears.
- Users who want content from the historical system re-upload supported source
  files through the normal bounded Go ingestion path; no database conversion is
  promised.
