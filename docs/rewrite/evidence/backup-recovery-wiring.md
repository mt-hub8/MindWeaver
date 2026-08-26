# Backup startup recovery integration evidence

Status: candidate; this file does not mark `BKP-001` or a release gate PASS.

## Frozen inputs

- Product baseline: `codex/go-rewrite@9b476166b547ef088e566cb56f1243558e08e12f`.
- Selected kernel commits only:
  - B1 `8fda3199261db396e4530f8830bb258c86c90a83` (standalone verify and clean-machine restore).
  - B2 `1e9345d8a2ad4aa7f009b26982f8a552f54d573c` (startup residue inspection/recovery).
- The divergent candidate branch was not merged. No Java/MySQL migration, third PE,
  `migrate` command, live recovery HTTP route, or control/fake Vault was selected.

## Product boundary

The only product entry points are mutually exclusive startup commands:

```text
mindweaver recovery verify  -backup <backup-directory>
mindweaver recovery restore -backup <backup-directory> -vault <new-vault-directory>
```

Dispatch occurs before `app.Start`; therefore recovery mode does not open a
configured Vault, acquire its lock, bind the normal loopback listener, or start
workers. Verify uses an application-owned scratch directory below
`os.UserCacheDir()/MindWeaver/recovery/verify` (`LocalAppData` on Windows).
Restore binds an existing fixed-local destination parent and requires the final
direct-child Vault to be absent. It never merges or overwrites.

Startup cleanup is bounded and capability/CAS guarded. Verification cleanup
accepts only verification scratch residue. Restore cleanup validates an entire
bounded observation before mutation, accepts only inactive staging or
receipt-only evidence for the exact requested target, and blocks on truncation,
other kinds/targets, conflicts, or publication-uncertain evidence.

Command stdout is newline-delimited JSON containing only stable operation,
phase, aggregate counts/bytes, outcome, and failure class. It contains no input
path, URL, Vault path, source content, or database content. Normal CLI stderr
uses `apperror.PublicMessage`. Backups are explicitly plaintext; integrity is
provided by the manifest and artifact hashes, not confidentiality.

## Verification contract

Windows tests construct a real fresh Vault and SQLite database, create a real
backup, run product verify and restore, and reopen the restored Vault/database.
Canary assertions cover output and error surfaces and ensure recovery dispatch
does not create normal configuration. Kernel tests additionally cover retained
directory identity, no-overwrite, source/target overlap, corrupt input,
publication uncertainty, bounded cleanup, stale CAS, and path-free errors.

Required commands use the frozen Go 1.27 toolchain with `GOTOOLCHAIN=local` and
`GOPROXY=off`:

```text
go test ./internal/backup ./cmd/mindweaver
go test ./...
go vet ./...
powershell -ExecutionPolicy Bypass -File scripts/verify-standalone.ps1 -Go <go1.27>
```

## Remaining product evidence

This slice deliberately does not add normal backup creation UI/API, scheduled
backup policy, release/MSI logic, or clean-machine installer rehearsal. Those
separate artifacts remain required before `BKP-001` or release acceptance can
be promoted. Plaintext backup placement and retention remain operator-owned.
