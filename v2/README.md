# MindWeaver v2 platform and data kernel

This directory is an independently extractable Go module. It does not import,
modify, or build the legacy Java application.

- Module: `github.com/mt-hub8/MindWeaver/v2`
- Configuration contract: `mindweaver.v1.json` (`schema_version: 1`)
- Shared boundaries: `platform`, `platform/apperror`, `platform/event`, `job`
- Dependency policy: standard library only until a SQLite driver ADR and proof
  are accepted

## Run

```powershell
$env:MW_GO = 'C:\path\to\go.exe'
& $env:MW_GO run ./cmd/mindweaver version
& $env:MW_GO run ./cmd/mindweaver config init -file mindweaver.v1.json -vault ./vault
& $env:MW_GO run ./cmd/mindweaver config check -file mindweaver.v1.json
```

The CLI refuses unknown configuration fields, unsupported schema versions, and
overwriting an existing configuration file.

## Verify fully offline

The verification entry points force `GOTOOLCHAIN=local`, `GOPROXY=off`,
`GOSUMDB=off`, and read-only module resolution. They format-check, test, vet,
and build without external services.

```powershell
./scripts/ci.ps1 -Go C:\path\to\go.exe
./scripts/verify-standalone.ps1 -Go C:\path\to\go.exe
```

```sh
MW_GO=/path/to/go ./scripts/ci.sh
MW_GO=/path/to/go ./scripts/verify-standalone.sh
```

The nested `.github/workflows/ci.yml` becomes an active workflow when `v2/` is
extracted as its own repository. In the current monorepo, the scripts are the
authoritative CI entry points.

## Transaction boundary

The public `job.Store` contract supports short optimistic writes. The intended
execution sequence is: claim in a short transaction, call an external provider
outside any transaction, then finalize with another short compare-and-swap.
