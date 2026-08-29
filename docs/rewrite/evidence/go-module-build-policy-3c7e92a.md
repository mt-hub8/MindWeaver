# Checksum-verified Go module build policy

Status: **current build boundary; not a release qualification**

## Decision

Exact code baseline
`3c7e92a1848ee7e37e579467428d99f7a7f93e6a` removes the checked-in
`v2/vendor/` tree and makes `go.mod` plus `go.sum` the dependency authority.
MindWeaver remains a local-first Windows application; allowing dependency
downloads during development or CI does not add runtime network authority.

A cold build or cache miss requires network access to a Go module proxy and
checksum service. CI may reuse the module cache. Every build first runs
`go mod download` and `go mod verify`, then uses `-mod=readonly` so a gate
cannot rewrite dependency declarations. The pinned Go 1.27.0 toolchain,
Windows/amd64 target, CGO-disabled production shape, trimpath, and disabled VCS
stamping remain unchanged.

## Repository and dependency contract

- `v2/qualification/moduleintegrity` rejects a checked-in vendor directory,
  freezes the exact `go.mod` and 18 `go.sum` h1 records, checks the exact root
  and extracted workflow sets, and rejects vendor or network-disabled build
  policy regressions.
- Root and extracted workflows pin checkout and setup-go actions, enable the
  Go module cache keyed by `go.sum`, and run download, verification, full CI,
  and tracked-only standalone extraction.
- Nested Go subprocesses use the same readonly module policy. Production
  buildinfo must contain the exact module version and h1 set rather than the
  empty sums produced by vendor mode.
- Supply-chain qualification reads license and notice evidence from the module
  cache, verifies raw file hashes, and binds each dependency to the committed
  module h1. Module h1 values are not misrepresented as raw SHA-256 values.
- The shipped artifact exact set remains `mindweaver.exe` and
  `mindweaver-pdf.exe`.

## Deletion preservation

Before deletion, the exact 698-file vendor tree was archived outside the
repository as logical archive `v2-vendor-head.zip`. Its SHA-256 is
`feb9e6ed816d5165fbea348c9b89d0aedb59c6b633e449e3b52fcd2ff5d36cbe`.
The same receipt directory contains the preflight Git status, staged and
unstaged patches, the vendor Git-tree listing, and a SHA-256 manifest. The
archive is recovery evidence, not a build input.

The repository change removes 566,071 vendored lines. This makes the V2 pull
request primarily first-party implementation, tests, contracts, and review
evidence instead of automatically generated third-party source.

## Verified identities

Two independent build-cache runs produce:

- `mindweaver.exe`: source manifest
  `3f3cf6de017bae3aa0af4a939baade6f626a503302a3060b575f6066151ac01a`,
  PE SHA-256
  `f2c63996741bf6306b8c48a251464961b78e99f7b42bdf07402253de1e511de2`;
- `mindweaver-pdf.exe`: source manifest
  `0bec9ddde1ea8778ffc3c20740ed55080d7ef0b537cfd070a2d563ccc5a087c6`,
  PE SHA-256
  `b9cc03b7c139e9fadde86f2ec9564dbe368d85822bd96ac64385e7aa75827384`;
- deduplicated source union:
  `34ccd6e3ab8b15575eef652c16052552b7c14bd067e1ec7c7a62467f655084ff`.

The ordinary full test, vet, module-integrity, PDF, production, reliability,
runtime, browser-contract, and supply-chain packages pass under this policy.
Final `ci.ps1` and tracked-only `verify-standalone.ps1` are required again on
the committed evidence tree.

## Non-claims

`go.sum` verifies downloaded module content; it does not select the project's
license, approve legal conclusions, sign artifacts, produce a final SBOM or
NOTICE, or qualify an MSI and clean-machine lifecycle. Those remain REL-002
and SEC-002 work and are outside the Java-domain/backend completion boundary.
