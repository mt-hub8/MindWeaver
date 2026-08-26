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
workers. Verify resolves `FOLDERID_LocalAppData` with the Windows Known Folder
API rather than trusting `%LOCALAPPDATA%`, then uses the dedicated direct child
`MindWeaver-Recovery-Verify-v1`. Before its first write, the implementation
retains the backup source and Known Folder identity, rejects ancestor overlap,
atomically creates the scratch leaf through the retained parent, and requires
the leaf to have the current user as owner plus a protected owner-only DACL.
The constructor, every cleanup pass, and standalone verification revalidate
the actual retained scratch identity and DACL before mutation. Restore binds an
existing fixed-local destination parent and requires the final direct-child
Vault to be absent. It never merges or overwrites.

Startup cleanup is bounded and capability/CAS guarded. Verification cleanup
accepts only inactive staging or receipt-only residue whose kind and destination
are exactly the fixed verification scratch target. Restore cleanup retains its
backup source before scanning, validates an entire bounded observation before
mutation, accepts only inactive staging or receipt-only evidence for the exact
requested target, and applies an identity/ancestor guard to every exact staging
leaf before deletion. It allows a normal backup and new Vault to be siblings,
but blocks any writable leaf equal to, inside, or containing the source. Both
paths block on truncation, other kinds/targets, conflicts, or
publication-uncertain evidence.

Command stdout is newline-delimited JSON containing only stable operation,
phase, aggregate counts/bytes, outcome, and failure class. It contains no input
path, URL, Vault path, source content, or database content. Normal CLI stderr
uses `apperror.PublicMessage`. Backups are explicitly plaintext; integrity is
provided by the manifest and artifact hashes, not confidentiality.

## Verification contract

Windows tests construct a real fresh Vault and SQLite database, create a real
backup, run product verify and restore, and reopen the restored Vault/database.
Canary assertions cover output and error surfaces and ensure recovery dispatch
does not create normal configuration. The CLI tests inject only their private
temporary scratch capability; production still resolves the Windows Known
Folder. Separate Windows tests prove that hostile `%LOCALAPPDATA%` does not
change Known Folder resolution, source-containing scratch is rejected before
any write, existing or newly created scratch is owner-only, another verify
target is retained, and sibling-source restore remains usable. Kernel tests
additionally cover retained directory identity, no-overwrite, source/target
overlap, corrupt input, publication uncertainty, bounded cleanup, stale CAS,
and path-free errors.

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
