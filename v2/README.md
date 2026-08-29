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
release, durability, security, and platform qualification matrix. CORE Search
and Ask accept one literal continuous source phrase of at least three Unicode
code points and at most 1024 UTF-8 bytes. The complete trimmed Search input, or
the complete unpadded Ask input, is searched unchanged; there is no word
segmentation, term OR, query rewriting, synonym expansion, or semantic search.
Two-code-point queries are unsupported. Ask is therefore keyword/phrase-driven
grounded generation, not general natural-language question answering. Future
natural-question retrieval quality and its recall/ranking/FDR/capacity budgets
are `RET-003` LATER and remain unimplemented; every previous candidate has
`Selection=NONE`. Startup-only standalone backup verification and
no-active-Vault restore/reopen have passed the `BKP-001` development
acceptance. Packaged clean-VM backup/restore lifecycle, long-running SQLite
stress, real browser qualification, packaging, and release qualification remain
incomplete. Live no-replace backup creation and startup-only verify/restore are
accepted CORE behavior, but their presence must not be read as an installed,
signed, or otherwise supported release promise.
The product intentionally creates a fresh Vault and exposes no Java/MySQL data
import command or compatibility path.

The isolated `spikes/sqlite` module and its `run.ps1` are historical dependency
qualification evidence, not a production storage implementation or a CUT-002
delivery/standalone entry point. They are not invoked by the gates below.

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
& $env:MW_GO mod download
& $env:MW_GO mod verify
& $env:MW_GO build -mod=readonly -trimpath -buildvcs=false -o dist\mindweaver.exe ./cmd/mindweaver
& $env:MW_GO build -mod=readonly -trimpath -buildvcs=false -o dist\mindweaver-pdf.exe ./cmd/mindweaver-pdf
```

Relative Vault paths are resolved next to the configuration file so the same
configuration cannot silently select a different Vault when launched from a
shortcut or another working directory.

## Verify

```powershell
./scripts/ci.ps1 -Go C:\path\to\go.exe
./scripts/verify-standalone.ps1 -Go C:\path\to\go.exe
./scripts/test-browser.ps1 -GoExecutable C:\path\to\go.exe -SelfTest
```

```sh
MW_GO=/path/to/go ./scripts/ci.sh
MW_GO=/path/to/go ./scripts/verify-standalone.sh
```

The `.sh` entry points above target Windows Git Bash. They intentionally set
`GOOS=windows` and run the repository's Windows qualification; they are not a
claim that the application or its Windows tests run natively on Linux.

The scripts format-check, test, vet, and build with the exact Go 1.27.0
toolchain while `go.mod` retains its Go 1.26 language/module directive. They
freeze `GOAMD64=v1` with no optional Go experiments and FIPS mode off, run
`go mod download` plus `go mod verify`, and force `-mod=readonly` so gates
cannot rewrite dependency declarations. Module acquisition may use the
network and Go's module cache; `go.sum` binds downloaded content. Build and
temporary caches remain disposable. The browser self-test uses the same
readonly module policy and restores every Go environment variable it changes.

Standalone verification accepts either this `v2/` directory in the monorepo or
the root of an extracted repository. In both layouts it requires a clean
committed tree and builds a tracked-only `git archive` (`HEAD:v2` or `HEAD`),
which contains no `.git` metadata. It compares the archive's exact path set
with the Git tree and fails unless `scripts/ci.sh` and
`scripts/verify-standalone.sh` are the only `100755` entries. The nested
`.github/workflows/ci.yml` becomes active after extraction and pins Go 1.27.0
explicitly. A passing development gate is necessary evidence, not release
qualification.

In the monorepo, root `.github/workflows/go-ci.yml` is the only active
workflow and enters `v2/`; the retired Java/MySQL/Rabbit build is not run.
Both repository layouts pin checkout and Go setup actions to immutable commit
SHAs. GitHub downloads those actions, the pinned Go bootstrap, and missing Go
modules, and caches modules using `go.sum` as the dependency key. This is a
build-time supply-chain boundary, not runtime product traffic. Each workflow
declares its checked-out Git root explicitly; an extracted checkout validates
only its own workflow directory and never inspects a parent.

## Closed local workflow

```text
upload -> immutable blob -> durable ingestion job -> SQLite chunks/FTS5 -> search
                 |-> persistent status -> explicit retry/cancel
document -> collection M:N -> trash/restore -> durable permanent cleanup
```

The automated app and workbench tests prove this result survives process
restart, a second process cannot own the same Vault, and half-built revisions
never become searchable.
