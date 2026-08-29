# REL-002 SQLite 3.53.4 upstream provenance

Status: **qualified pre-package provenance slice; REL-002 remains
NOT_IMPLEMENTED**

- Code commit: `e8cf45db8482504423c23e17a013f2ba1e2f1dfa`
- Recorded: 2026-08-28 (Asia/Shanghai)
- Executable contracts:
  `v2/qualification/supplychain/{inventory_test.go,sqlite_upstream_test.go}`
- Authority-owned bytes:
  `v2/third_party/sqlite/3.53.4/{manifest.uuid,LICENSE.md}`
- Scope: upstream identity and public-domain provenance for the SQLite 3.53.4
  material translated into the vendored Go runtime. This is not a legal
  conclusion, translation-reproducibility proof, SBOM, NOTICE or package.

## Controlled acquisition

On 2026-08-28, the official canonical source archive was acquired outside the
repository from:

`https://www.sqlite.org/2026/sqlite-src-3530400.zip`

The official SQLite download page identifies that archive as the complete
canonical source tree (the source "urtext") for 3.53.4 and publishes its
SHA3-256. Independent full-file hashing reproduced the published value:

`b834d474b9b393d85a9e3ee4cc11f1329e007e9376a424ee740796f5c4bda3a8`

Following SQLite's canonical-source documentation, the manifest checksum
reproduced this source UUID:

`bf7c7f30031888f4e796e429ab3978879485813aaca6f641c7b33e4e09459bcc`

That value exactly matched the extracted `manifest.uuid`. The same manifest
contained exactly this reviewed artifact entry:

`F LICENSE.md 6bc480fc673fb4acbc4094e77edb326267dd460162d7723c7f30bee2d3d9e97d`

SHA3-256 over the extracted `LICENSE.md` matched that Fossil artifact ID. The
two committed authority-owned files were compared byte-for-byte with the
members from the hash-verified archive. Their executable-contract raw hashes
are:

| File | Raw SHA-256 |
| --- | --- |
| `LICENSE.md` | `ee6af51062b30d532991face5164136ae6f84e265ecf8abe89dc69dac45ca1e7` |
| `manifest.uuid` | `c613b5f6581cba368618c41ccfc16e476f65efa963c9c7c9fc3b4519daea71d9` |

The archive was an acquisition input only. It is not a shipped product file,
is not read by production or CI, and is not substituted for the committed
bounded evidence. The acquisition can be reproduced from the official
[download page](https://sqlite.org/download.html),
[release history](https://www.sqlite.org/changes.html),
[canonical-source instructions](https://www.sqlite.org/getthecode.html), and
[public-domain statement](https://sqlite.org/copyright.html).

## Executable closure

`TestPrepackageSupplyChainInputClosure` now fails before blocker assessment
unless all of the following form one successful chain:

1. `third_party/sqlite` has the exact version set `{3.53.4}` and the version
   directory has exactly the two bounded regular non-link files above;
2. raw size/SHA-256, the LICENSE Fossil artifact ID, lowercase UUID plus LF,
   and the reviewed public-domain scope for primary SQLite source, extensions,
   and `sqlite3.c`/`sqlite3.h` build products match;
3. the hash-bound vendored `go-sqlite3-wasm/v3` README still states that the
   original authors and original licenses remain in effect, and its module
   LICENSE remains the separately scoped MIT-0 evidence;
4. the vendored production driver, inside the qualification process, returns
   `sqlite_version() = 3.53.4` and the exact source ID
   `2026-07-24 19:02:57 bf7c7f30031888f4e796e429ab3978879485813aaca6f641c7b33e4e09459bcc`;
5. the same pre-package test builds the real Windows `mindweaver.exe` offline
   and confirms from its build information that it links
   `github.com/ncruces/go-sqlite3-wasm/v3@v3.2.35304`.

The runtime query executes in the qualification test process, not by issuing a
query inside the built PE. Together with the hermetic vendored build and PE
module mapping, it proves the reviewed vendored runtime input identity; it does
not claim a reproducible SQLite-to-Wasm-to-Go translation toolchain.

Fail-closed mutations cover the LICENSE bytes, extra evidence file, extra
upstream version, a directory replacing a required regular file, wrong runtime
version, wrong runtime source ID, wrong manifest UUID binding, missing vendor
retention semantics, missing upstream public-domain scope, and a wrong Fossil
artifact ID.

## Blocker delta and non-claims

The narrow executable pre-package blocker exact set is now only:

- `PROJECT_LICENSE_MISSING`.

The earlier fixed-baseline evidence in `rel002-prepackage-input-592339e.md` and
`prepackage-dual-pe-output-982a36e.md` remains historical. This document only
supersedes their statement that SQLite upstream provenance was missing.

This slice does not prove translation equivalence, choose a project license,
or emit a final SBOM/NOTICE, vulnerability result, MSI, signature, ICE report,
offline-egress attestation or clean-VM lifecycle evidence. Therefore REL-002
remains `NOT_IMPLEMENTED`.

## Validation

The exact clean code commit `e8cf45db8482504423c23e17a013f2ba1e2f1dfa`
passed, sequentially, with the repository-selected Go 1.27.0 toolchain:

- `go test ./... -count=1`;
- `go vet ./...`;
- `scripts/ci.ps1`, including empty-cache vendored/offline tests and both
  Windows PE builds;
- `scripts/verify-standalone.ps1`, from its tracked-only archive with no
  parent repository dependency.

The evidence/ledger change also passed the 705-row inventory validator, the
866-file legacy manifest validator, and `git diff --check`. These results keep
the already-qualified extracted-build boundary intact; they do not promote
REL-002.

## Reproduction

From `v2/` with the repository-selected Go 1.27.0 toolchain and the frozen
vendored/offline environment:

```powershell
go test ./qualification/supplychain -run '^(TestPrepackageSupplyChainInputClosure|TestPrepackageInventoryStructuralMutationsFailClosed|TestSQLiteTranslationUpstreamProvenanceMutationsFailClosed)$' -count=1 -v
go vet ./qualification/supplychain ./qualification/offlinevendor
./scripts/verify-standalone.ps1 -Go <absolute-go1.27.0>
```
