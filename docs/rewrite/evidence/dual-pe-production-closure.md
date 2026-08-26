# Dual-PE production dependency closure

Status: **CUT-002 candidate qualified on its frozen branch; not integrated and
not release-qualified**

## CUT-002 vendored extraction candidate

This final adaptation is based on exact committed main
`3c2e4637b67ab3563389668472b9442454c3492e`. It selectively replays
`c2871b1`, `583536da3beddfdb7fbb0d39fc5f7734ea4cc25e`, and
`7dddb2a2c09bff5e2f60ce54320b8524c236ca27` as `4bed0dc`, `4b8fb30`, and
`686e5d3`, then refreshes the identities and extraction evidence in one
adaptation commit. The final adaptation hash is reported by the handoff rather
than self-referenced inside its own contents. It is not merged and does not
claim the identity of a later controller branch.

The replay preserves the current RET boundary corpus
`core-keyword-boundary.8aadf60.v2.json` and the current evidence-only runtime
helper classification. Neither conflict adds a shipped command: the exact
production command set remains `mindweaver.exe` and `mindweaver-pdf.exe`.
The final qualification-only follow-up also puts the runtime interruption
harness's nested Go builds into vendor mode and makes its Windows child-marker
reader tolerate only a bounded publication interval. Those changes are test
infrastructure, not production-source inputs, and do not change either PE
identity.

The frozen build uses Go 1.27.0 on Windows/amd64 with `CGO_ENABLED=0` while
`go.mod` remains at `go 1.26`. Every module-resolving Go subprocess is forced to
`-mod=vendor` under the offline controls, and every shipped build uses
`-trimpath -buildvcs=false`. `GOAMD64=v1`, an empty `GOEXPERIMENT`,
`GOFIPS140=off`, and `GOTELEMETRY=off` are explicit. `GOPROXY`, `GOSUMDB`,
toolchain resolution, workspaces, and VCS lookup are disabled. Each CI
invocation creates new empty `GOMODCACHE`, `GOCACHE`, and `GOTMPDIR`
directories and rejects any module-cache write.

The resulting exact identities are:

- `mindweaver.exe`: 77 first-party source files,
  `ba61947c93896ac083c0517f9e10df8f6e540eb7fda5074389363169af17ff06`;
  PE SHA-256
  `ca7c56bb8ec1144f989108d3f06d09e75a79a4a5aa0f4bdb928ba9e1f6cf7316`
- `mindweaver-pdf.exe`: 5 first-party source files,
  `0bec9ddde1ea8778ffc3c20740ed55080d7ef0b537cfd070a2d563ccc5a087c6`;
  PE SHA-256
  `a2a6a04b4ade9367aab6cce35e9a3c87351f9c8fabd33d9ea2fe108f7e732241`
- deduplicated dual-PE union: 81 first-party source files,
  `73aca40cc25a29477f51559a9a3bd23180ee124a78e0f4a133ca663987f50cfc`

Vendored buildinfo deliberately carries exact module path and version with an
empty `Sum`. The independent vendor contract restores the trust chain by
binding all eight module `h1` values from `go.sum`, the exact 37-package
`vendor/modules.txt` declaration, the canonical 698-file vendor tree
(`f935ae79254d0fd1f5f32f4491f0b1f1e28e22fc088cef262bde2e860cf9c254`),
and all eight dependency `LICENSE` files plus `golang.org/x/sys/PATENTS`.
Module replacements and unreviewed legal files fail closed. Vendor directories
are excluded from first-party package, main-package, source-file, and union
statistics.

Git whitespace diagnostics exempt only the byte-preserved canonical vendor
tree. A temporary-repository regression test proves the same trailing-space
mutation is ignored under `vendor/` but still makes `git diff --check` fail for
a first-party file; no upstream vendor byte is rewritten to satisfy that gate.

At the final adaptation checkpoint reported by the handoff, both supported
repository layouts are exercised with the same Windows toolchain and offline
controls:

- monorepo `HEAD:v2` tracked-only archive: full tests, vet, dual-PE builds, and
  empty-module-cache assertion **PASS**;
- locally cloned extracted repository-root `HEAD` tracked-only archive: the
  same complete gate **PASS**.

Both inner archives are generated only from clean committed trees and reject
`.git` metadata. The gate compares the archive's exact path set with the Git
tree, accepts only ordinary blob modes, and requires the exact `100755` set to
be `scripts/ci.sh` and `scripts/verify-standalone.sh`; this closes the earlier
lossy temporary-Git reconstruction evidence. Git whitespace diagnostics still
exclude only canonical vendor bytes, with a regression proving a first-party
whitespace defect fails. These results qualify the candidate's
build/extraction boundary; they do not close the project LICENSE, SBOM, MSI,
signing, or clean-VM release blockers.

## Earlier non-vendored integration refresh

The original qualification below remains immutable historical evidence for
baseline `d61a158`. A later non-vendored integration reran it after the reviewed
backup, RAG, browser-test, knowledge, and dual-PE slices. Its then-current exact
values were:

- `mindweaver.exe`: 77 source files,
  `3b1f0355cf4498f0b79c7cb1fe9644eb7741a6bbabc8fe4056fd38f926353ad0`;
  PE SHA-256
  `80a54f16d0d6786ee3f9d202e49110a6101f6f23de64e8ce9e88656e20eed23e`
- `mindweaver-pdf.exe`: 5 source files,
  `0bec9ddde1ea8778ffc3c20740ed55080d7ef0b537cfd070a2d563ccc5a087c6`;
  PE SHA-256
  `b9cc03b7c139e9fadde86f2ec9564dbe368d85822bd96ac64385e7aa75827384`
- deduplicated dual-PE union: 81 source files,
  `3efba79e422531a2afcce6d0b311b847254455b108a3a41a86e8240b7cf8e6e4`

At that integration point, `openapi/v1` bound the refreshed `mindweaver.exe`
source manifest. Its
production discovery policy also classifies `tests/browser/runner` as an
evidence-only main package, so that qualification code cannot silently become
a shipped command. Focused OpenAPI and dual-PE tests pass on the integrated
tree. The historical CI failure and hashes retained below describe only the
original `d61a158` run; they are not the current CUT-002 candidate status.

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
scans the final linker symbol table. Each PE SHA-256 is an executable assertion,
and the gate rebuilds each command with two separate initially empty `GOCACHE`
directories before accepting reproducibility. It also freezes the complete
reachable first-party package exact-set for each command. It does not rely only
on source imports or on a few forbidden-package spot checks. These hashes bind
this exact integrated source tree; a later WebUI or other production-source
change must fail the gate and be explicitly reviewed and re-frozen.

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

## Historical baseline findings

- **Resolved at that historical integration point:** the committed `mindweaver`
  production source-manifest digest has been refreshed, and the full CI and
  standalone verification are rerun before accepting each new production
  source identity.
- **P1:** linker symbol spellings are deliberately pinned to Go 1.27.0. A Go
  toolchain change requires an explicit gate review rather than silent drift.
- This run proves offline resolution from the locally available module cache;
  it does not claim an empty-cache or vendored build.

Only test and evidence files are changed by this qualification slice. No
OpenAPI, backup, WebUI, release, migration, vendor, or production source is
modified.
