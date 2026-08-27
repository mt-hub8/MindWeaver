# REL-002 pre-package supply-chain input boundary

Status: **NOT_IMPLEMENTED**

- Code commit: `592339e1d6eef911e199f6c1c713ec03ece0d912`
- Recorded: 2026-08-28 (Asia/Shanghai)
- Executable contract:
  `v2/qualification/supplychain/inventory_test.go`
- Scope: exact offline inputs required before package generation; no package,
  SBOM, NOTICE, signature or release artifact is emitted.

## What is closed

The qualification builds the real `mindweaver.exe` and
`mindweaver-pdf.exe` with the repository-selected Go 1.27.0 toolchain, the
vendored module tree and network-disabled module resolution. It reads each PE's
real Go build information and requires the exact command main path, no module
replacement and the reviewed per-PE module mapping.

For every shipped module it independently binds the exact version and root
`go.sum` h1, then verifies the expected vendored regular files by raw byte size
and SHA-256. The inventory includes the relevant module LICENSE files,
`golang.org/x/sys/PATENTS`, the SQLite translation README statement, and the Go
runtime `LICENSE`, `PATENTS` and `VERSION`. The Go module h1 is kept distinct
from a raw-file SHA-256 and is never placed into a SHA-256 field.

The contract also rejects structural mutations including an unknown module,
the wrong PE-to-module mapping, a module replacement, the wrong command main
path and an h1 value masquerading as a raw SHA-256. The offline build must leave
its initially empty `GOMODCACHE` empty.

SPDX labels in this contract are reviewed candidates for later inventory
generation, not legal conclusions and not a project-license selection.

## Exact current blockers

The executable pre-package assessment requires the exact sorted blocker set:

- `PROJECT_LICENSE_MISSING`;
- `SQLITE_TRANSLATION_UPSTREAM_PROVENANCE_MISSING`.

The first blocker requires an authorized project LICENSE at the extracted
product root. Third-party licenses cannot substitute for it. The second
requires hash-bound upstream SQLite 3.53.4/public-domain or blessing provenance
for the translated SQLite/FTS material; the module's MIT-0 file and README alone
do not assert coverage of all translated upstream code.

These are the blockers visible to this narrow pre-package input contract. They
are not the full release-blocker inventory. Gate 8 in
`docs/rewrite/quality-gates.md` and the accepted Windows runtime/delivery ADRs
remain authoritative for that wider boundary. It additionally includes the
approved offline WiX toolchain and reviewed EULA evidence, trusted signing
key/certificate and offline Authenticode verification, ICE validation, real
MSI generation, distinct N-1/N-2 package lineages, clean non-admin VM install/
launch/upgrade/rollback/uninstall and retained-Vault verification, packaged
clean-machine backup restore/reopen, package-level offline-egress proof, final
SBOM/NOTICE and vulnerability gate, and the release-wide bounded artifact/
transcript scan.

## Deliberate non-claims

This evidence does not generate or validate SPDX or CycloneDX output, a NOTICE,
portable archive, MSI, signature, release index or clean-machine attestation.
It does not accept an EULA, install or download WiX, create or trust a
certificate, or claim legal compatibility. Neither passing this test nor the
presence of vendored license files can promote `REL-002`; the row remains
`NOT_IMPLEMENTED`.

## Reproduction

From `v2/` with the repository-selected Go 1.27.0 toolchain and the frozen
offline module environment:

```powershell
go test ./qualification/supplychain -run '^(TestPrepackageSupplyChainInputClosure|TestPrepackageInventoryStructuralMutationsFailClosed)$' -count=1 -v
go test ./qualification/offlinevendor -count=1
```
