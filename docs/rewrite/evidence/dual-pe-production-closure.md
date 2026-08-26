# Dual-PE production dependency closure

Status: **QUALIFIED; integrated source contract synchronized**

## Current integration refresh

The original qualification below remains the immutable evidence for baseline
`d61a158`. After integrating the reviewed backup, RAG, browser-test, knowledge,
and dual-PE slices, the same offline gate was rerun against the current Go
product tree. The refreshed exact values are:

- `mindweaver.exe`: 77 source files,
  `b7d8ca3e12648fd472c5f9eb1d774fe5aef7b91b694080b2a813380295ddc748`;
  PE SHA-256
  `e9415f981d538b7588610e4984d89d09dcc8867b7519d47abef80e4e22b8262c`
- `mindweaver-pdf.exe`: 5 source files,
  `bf8badaa11f215a4acd100a839d6e360017bdbc5d9ae18cbbb67e9222ab8849a`;
  PE SHA-256
  `411ed538b53d533600ae5466427f3d3f30a7a81405885d92b79167e52ff32b87`
- deduplicated dual-PE union: 81 source files,
  `0c8c2e0f101b632a073bea6600c103304f7ad48d824573242308e8e5e94e1d07`

`openapi/v1` now binds the refreshed `mindweaver.exe` source manifest. Its
production discovery policy also classifies `tests/browser/runner` as an
evidence-only main package, so that qualification code cannot silently become
a shipped command. Focused OpenAPI and dual-PE tests pass on the integrated
tree. The historical CI failure and hashes retained below describe only the
original `d61a158` run; they are not the current repository status.

## Bound inputs

- Baseline: `codex/go-rewrite@d61a158afdc7c86914c70a847ce4e9d6fe686517`
- Qualification commit: `d298d6cedbd0ee95749d3b337e25c338c8df0353`
- Toolchain: `go version go1.27.0 windows/amd64`
- Build target: `GOOS=windows`, `GOARCH=amd64`, `CGO_ENABLED=0`
- Offline controls: `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOSUMDB=off`,
  `GOWORK=off`, `GOFLAGS=-mod=readonly -buildvcs=false`
- Build flags: `-trimpath -buildvcs=false`

The gate is `TestWindowsAMD64ShippedDualPEClosure` in
`v2/qualification/production/dual_pe_test.go`. It creates both final PE files
in a new test-owned directory, checks the `MZ` header, reads Go buildinfo, and
scans the final linker symbol table. It also freezes the complete reachable
first-party package exact-set for each command. It does not rely only on source
imports or on a few forbidden-package spot checks.

The same frozen Windows build selection inventories every module-local
compiler input (`go`, cgo/native categories, syso, and embedded files),
canonicalizes text line endings, hashes every selected file, and binds both
per-command manifests plus their deduplicated source union:

- `mindweaver.exe`: 76 files,
  `7de68fba33368f4cfa74a44cb27a0c5add95c688f70c0c77b939bbb21d825dca`
- `mindweaver-pdf.exe`: 5 files,
  `bf8badaa11f215a4acd100a839d6e360017bdbc5d9ae18cbbb67e9222ab8849a`
- deduplicated dual-PE union: 80 files,
  `0d0999819779d9c7acde94fc734b70eb0b4d5defd315777cfc5e0547765eca48`

## Qualified exact-set

`go list ./cmd/...` contains exactly these two main packages, and the isolated
artifact directory contains exactly these two regular files:

| Artifact | Required linked boundary | Forbidden linked boundary | SHA-256 |
| --- | --- | --- | --- |
| `mindweaver.exe` | `internal/app`, `internal/backup`, PDF client/protocol | PDF parser and all `github.com/mgilbir/*` parser modules | `e19aed56e62869d98555527fb0a2d2034da70bb272e8cd97675657d03061d6a7` |
| `mindweaver-pdf.exe` | PDF parser/protocol and `github.com/mgilbir/pdf0` | `internal/app`, `internal/backup`, PDF client, `os/exec`, `x/sys/windows` | `411ed538b53d533600ae5466427f3d3f30a7a81405885d92b79167e52ff32b87` |

The gate also rejects `cmd/mindweaver-migrate`,
`cmd/mindweaver-neutral-export`, any third main package under `cmd`, and any
third file in the built artifact directory. Exact first-party reachability
rejects any unreviewed package, with explicit diagnostics for Agent, vector,
embedding, reindex, rerank, memory, notification, evaluation, and batch LATER
segments. Exact module reachability additionally emits explicit failures for
MySQL, MariaDB, Flyway, or Spring dependencies.

Exact `mindweaver.exe` external module closure:

- `github.com/ncruces/go-sqlite3@v0.35.3#h1:Ei07Zv1qfV/vyXzelhFsyS5Oh9TArBZHsmFk14Xv3GY=`
- `github.com/ncruces/go-sqlite3-wasm/v3@v3.2.35304#h1:5NoQAewtgKNK3G4bjNPxVoGXu6F6NzLXWCTdD5FFAEY=`
- `github.com/ncruces/julianday@v1.0.0#h1:fH0OKwa7NWvniGQtxdJRxAgkBMolni2BjDHaWTxqt7M=`
- `golang.org/x/sys@v0.47.0#h1:o7XGOvZQCADBQQ4Y7VNq2dRWQR7JmOUW8Kxx4ZsNgWs=`

Exact `mindweaver-pdf.exe` external module closure:

- `github.com/mgilbir/formalis@v0.3.1#h1:NyYe/EcRYJ2jUjgaZG98lNXgJ7H+jgy6mq7HXOnQxl8=`
- `github.com/mgilbir/golittlecms@v0.0.0-20260727161601-f6af7cfe1556#h1:2ZUsOgMhxpHCYC8jyzeEnJZFLYGbXhqjAJWJBvY8q4U=`
- `github.com/mgilbir/gopenjpeg@v0.0.0-20260727163526-8a139bc479b2#h1:kdDIM4JNxn9gsRk5Zo6mtmcFpBqnl9gTVUwf9t6lIRk=`
- `github.com/mgilbir/pdf0@v0.1.0#h1:rfBK18bcQ4kHQTXBmriAb07TafhG2w1fLflq9lHgaG4=`

No module replacement is accepted in either PE.

## Commands and results

Focused gate:

```powershell
$env:MW_GO = 'C:\Users\24281\AppData\Local\MindWeaver\toolchains\go1.27.0\bin\go.exe'
$env:GOTOOLCHAIN = 'local'
$env:GOPROXY = 'off'
$env:GOSUMDB = 'off'
$env:GOFLAGS = '-mod=readonly -buildvcs=false'
$env:GOWORK = 'off'
& $env:MW_GO test ./qualification/production -run TestWindowsAMD64ShippedDualPEClosure -count=1 -v
```

Result: **PASS**. Both PE hashes above came from this run.

`go vet ./...`: **PASS** with the same frozen offline environment.

`scripts/ci.ps1 -Go $env:MW_GO`: **BLOCKED**. Every reported package except
`openapi/v1` passed, including the new production qualification. The sole
failure was:

```text
command mindweaver transitive source manifest SHA-256 =
7de68fba33368f4cfa74a44cb27a0c5add95c688f70c0c77b939bbb21d825dca,
contract = 325dfec05db7f7744c6aaf4de792d80efaa6b458a0b850a635d3c41e3596e099
```

`scripts/verify-standalone.ps1 -Go $env:MW_GO`: **BLOCKED by the same sole
failure** after copying the module to a fresh standalone directory.

To prove this was not introduced by the qualification package, a second clean,
detached worktree at exact baseline `d61a158` ran:

```powershell
& $env:MW_GO test ./openapi/v1 -run '^TestEmbeddedContractMatchesProduction$' -count=1 -v
```

It reproduced the identical `7de68f...` versus `325df...` failure without the
qualification commit. Updating the OpenAPI/core-surface digest is outside this
slice's authorized file boundary, so the repository-wide result remains
fail-closed rather than being reported as PASS.

## Open items

- **P0 outside this slice:** refresh the committed `mindweaver` production
  source-manifest digest on the final integrated tree, then rerun full CI and
  standalone verification.
- **P1:** linker symbol spellings are deliberately pinned to Go 1.27.0. A Go
  toolchain change requires an explicit gate review rather than silent drift.
- This run proves offline resolution from the locally available module cache;
  it does not claim an empty-cache or vendored build.

Only test and evidence files are changed by this qualification slice. No
OpenAPI, backup, WebUI, release, migration, vendor, or production source is
modified.
