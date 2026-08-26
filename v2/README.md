# MindWeaver Go rewrite

`v2/` is an independently buildable Go replacement for the legacy Java
application. The target is a single-user, loopback-only local knowledge
workbench backed by one Vault. It does not import or execute Java, MySQL,
RabbitMQ, or Python workers.

The rewrite follows a salvage-first rule: old Java behavior and already-written
Go code are retained only when they prove user value and close a real vertical
workflow. See the repository-level
[Go salvage review](../docs/rewrite/go-salvage-review.md).

## Current state

The executable now opens and exclusively locks one Vault, migrates SQLite,
reconciles stale staging files, expired jobs, and pending object cleanup, then
binds an ephemeral `127.0.0.1` port. Its dependency-free embedded Chinese UI
implements the local TXT/Markdown path from a bounded 4 MiB upload through a
durable ingestion job to scoped FTS5 search. Document and collection catalogs
use stable pagination; the current ingestion state survives restart and can be
explicitly retried after failure or cancellation. Real collection membership
and revision-checked trash, restore, and permanent-cleanup workflows are also
reachable from the UI.

A separately built `mindweaver-pdf` helper enables bounded text-PDF ingestion
only after it passes a versioned process probe. A missing, ordinary, or
incompatible file is reported as unavailable without disabling TXT/Markdown.

This is still a development build, not a qualified release. The implemented
paths above have automated evidence, but they have not passed the complete
release, durability, security, and platform qualification matrix. Ask/citation
architecture and structural citation checks are implemented, but natural-question
retrieval quality remains explicitly blocked by `RET-003`. The final retained
kernel-path boundary for backup destinations, long-running SQLite stress, real
browser qualification, packaging, and release qualification also remain
incomplete. Live no-replace backup creation and startup-only verify/restore are
implemented candidates, but their presence must not be read as a supported
release promise.
The product intentionally creates a fresh Vault and exposes no Java/MySQL data
import command or compatibility path.

The isolated `spikes/sqlite` module is dependency qualification evidence, not a
production storage implementation.

## Run the local workbench

```powershell
$env:MW_GO = 'C:\path\to\go.exe'
& $env:MW_GO run ./cmd/mindweaver version
& $env:MW_GO run ./cmd/mindweaver serve -config mindweaver.v1.json -vault ./vault
```

The first `serve` creates the versioned configuration if it is absent. Open the
one-use loopback URL printed by the process; the bootstrap credential is held
only in the URL fragment and exchanged for an in-memory session. Press
`Ctrl+C` to stop cleanly. Running with no command is equivalent to `serve`.

To exercise PDF ingestion from built artifacts, place both binaries together:

```powershell
& $env:MW_GO build -o dist\mindweaver.exe ./cmd/mindweaver
& $env:MW_GO build -o dist\mindweaver-pdf.exe ./cmd/mindweaver-pdf
```

Relative Vault paths are resolved next to the configuration file so the same
configuration cannot silently select a different Vault when launched from a
shortcut or another working directory.

## Verify

```powershell
./scripts/ci.ps1 -Go C:\path\to\go.exe
./scripts/verify-standalone.ps1 -Go C:\path\to\go.exe
```

```sh
MW_GO=/path/to/go ./scripts/ci.sh
MW_GO=/path/to/go ./scripts/verify-standalone.sh
```

The scripts format-check, test, vet, and build. A passing development gate is
necessary evidence, not release qualification. Module downloads are disabled
in these verification commands, so dependencies must already be present in the
Go module cache. The nested `.github/workflows/ci.yml` becomes active after
`v2/` is extracted into its own repository.

## Closed local workflow

```text
upload -> immutable blob -> durable ingestion job -> SQLite chunks/FTS5 -> search
                 |-> persistent status -> explicit retry/cancel
document -> collection M:N -> trash/restore -> durable permanent cleanup
```

The automated app and workbench tests prove this result survives process
restart, a second process cannot own the same Vault, and half-built revisions
never become searchable.
