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

The command-line bootstrap can print its version and create/check the small
versioned configuration. SQLite and Blob production stores are the first real
vertical-slice foundations under construction. There is not yet a released
server or usable document workflow, and this README deliberately does not claim
otherwise.

The isolated `spikes/sqlite` module is dependency qualification evidence, not a
production storage implementation.

## Run the bootstrap

```powershell
$env:MW_GO = 'C:\path\to\go.exe'
& $env:MW_GO run ./cmd/mindweaver version
& $env:MW_GO run ./cmd/mindweaver config init -file mindweaver.v1.json -vault ./vault
& $env:MW_GO run ./cmd/mindweaver config check -file mindweaver.v1.json
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

The scripts format-check, test, vet, and build. Module downloads are disabled in
these verification commands, so dependencies must already be present in the Go
module cache. The nested `.github/workflows/ci.yml` becomes active after `v2/`
is extracted into its own repository.

## First product gate

The next milestone is not another protocol package. It is one public workflow:

```text
upload -> immutable blob -> durable ingestion job -> SQLite chunks/FTS5 -> search
```

It is accepted only when the same result survives process restart and failure
tests prove that half-built revisions never become searchable.
