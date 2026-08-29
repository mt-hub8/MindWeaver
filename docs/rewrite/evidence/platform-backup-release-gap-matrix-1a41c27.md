# Platform, backup, release, and supply-chain gap matrix

Status: **evidence-only; no acceptance row is promoted**

Baseline: `1a41c27a76a74e8710587779610a4a7672370fa7`

Acceptance-ledger blob:
`910b4f760d90f427a0cfc6a5084aee6874c3a68b`

Machine-readable matrix:
[`platform-backup-release-gap-matrix-1a41c27.csv`](./platform-backup-release-gap-matrix-1a41c27.csv)

## Scope and ruling

This is a deletion-style read-only reconciliation of the CORE platform, data
kernel, backup, release, and supply-chain rows at the exact baseline above. It
does not alter production code, tests, scripts, workflows, vendor bytes, or the
acceptance ledger. Git blob identities in the CSV bind the reviewed evidence;
they are repository object IDs, not release artifact hashes.

No remaining row can honestly move to `PASS` in this slice. The repository-only
work that remains either belongs to an active incoming owner (`ARC-003`, DB),
must be repeated after the final production tree (`ARC-001`, `ARC-002`,
`SEC-002`, `CUT-002`), or would merely wrap partial qualifications in a
self-reporting gate (`REL-001`). The other open outcomes require external
authority, artifacts, hardware, time, or independently provisioned machines.
Creating another gate script would not create that evidence, so it is dropped.

## Repository boundary versus external boundary

### Closed in the repository

`ARC-001`, `ARC-002`, `RUN-001`, `RUN-003`, `RUN-004`, `CFG-001`, `DB-001`,
`JOB-001`, `JOB-002`, `PUR-001`, and `CUT-001` already have executable
repository evidence. Their next action is final regression, not another model,
protocol, migration command, or report generator.

The integrated CUT mechanics retain `go 1.26` in `go.mod`, use the frozen Go
1.27.0 Windows/amd64 toolchain, resolve modules only from canonical vendor, and
ship exactly `mindweaver.exe` plus `mindweaver-pdf.exe`. Those facts do not make
the current binary identities final and do not promote `CUT-002` here.

### Repository work deliberately deferred

- `ARC-003`: an ownership matrix is being reconciled by the ARC owner. A second
  matrix in this branch would create competing truth.
- `RUN-002`: the remaining COMMIT-in-flight point is unobservable without a
  narrow SQLite/VFS seam or a real machine fault. A generic transaction hook or
  journal would violate the lean local-core decision.
- `DB-001`: incoming DB work must preserve historical checksums and rerun the
  existing compatibility matrix; this evidence branch must not edit it.
- `DB-002`: a post-baseline bounded stress slice confirms that numeric SQLite
  `BUSY`/`LOCKED` is redacted at the HTTP boundary but still becomes
  non-retryable `INTERNAL`; a minimal serialized-writer or stable retryable
  product policy remains repository work before the external machine campaign.
- `SEC-002` and `CUT-002`: their remaining scans and identities are properties
  of the final integrated release candidate. Freezing them before SEC/DB/ARC
  lands would knowingly record stale evidence.
- `REL-001`: four real fuzz targets, production benchmarks, deterministic fault
  suites, and a bounded short soak exist. They are useful inputs, but a
  three-second soak is not a long soak and benchmark output without a controlled
  baseline is not a performance acceptance threshold.

### External or authority-owned blockers

| Input or evidence | Authority | Why the repository cannot synthesize it |
| --- | --- | --- |
| Project `LICENSE` decision | Project/legal owner | Selecting a project license is an ownership/legal decision; third-party license review is separate. |
| Approved offline WiX toolchain and accepted EULA | Release owner | Tooling must not download WiX or accept an EULA for the user. |
| Trusted signing certificate and private key | Release owner/CA | The repository must not create or install a signing identity. |
| Signed per-user MSI, Authenticode verification, and ICE results | Release pipeline | These exist only after the approved toolchain and certificate build the real package. |
| N-1 and N-2 signed MSI inputs | Release owner | Synthetic historical packages cannot prove upgrade or rollback compatibility. |
| Clean non-admin Windows install/upgrade/rollback/uninstall matrix | Independent VM runner | An in-worktree rehearsal is not a clean-machine result. |
| Packaged clean-machine backup restore/reopen | Independent VM runner | The current backup kernel is tested, but `BKP-001` explicitly requires the packaged product. |
| Physical ENOSPC/power-loss and 24-48h WAL/checkpoint/backup stress | Controlled hardware/VM owner | SQLite logical limits and process kills are not physical media faults or long-duration stress. |
| Blob K01B2/K05/K08 interruption points | Storage/DB owner or machine campaign | The current production boundaries cannot deterministically stop inside Sync, commit outcome, or unlink-to-directory-Sync. |
| Final SBOM/NOTICE and release-wide leakage scan | Release pipeline plus project license owner | The vendor input is frozen, but the final package, project license, logs, and signed manifests do not yet exist. |

At this baseline the repository contains eight reviewed third-party module
identities, eight `LICENSE` files and the `golang.org/x/sys` `PATENTS` file.
`v2/qualification/offlinevendor/vendor_test.go` rejects unknown modules,
replacements, missing sums, missing legal files, and license-byte drift. The
repository has no project `LICENSE`, project `NOTICE`, final SPDX/CycloneDX
SBOM, WiX sources, signing certificate, or signed MSI. This is a factual input
inventory, not a legal conclusion.

## Minimal next executable sequence

1. Integrate and independently review SEC/DB/ARC without using this evidence
   branch as a competing implementation.
2. On the resulting clean commit, regenerate the production source/union/PE
   identities and run both monorepo and extracted-root CUT gates with empty
   caches and network disabled.
3. Keep `BKP-001`, `REL-001`, and `REL-002` below `PASS` until their named
   external evidence is supplied. When it is supplied, bind reports to the
   exact signed artifacts and machine identities rather than to this baseline.
4. Continue to drop Java/MySQL migration, a migration command, a third PE,
   Agent/vector/LATER capabilities, built-in update machinery, and any gate
   that only declares its own success.

## Review findings

- **P0:** none found in this read-only scope.
- **P1:** the ledger correctly remains conservative, but `CUT-002` is now
  mechanically integrated while its evidence document still labels identities
  obsolete. This is intentional until the pending production integrations and
  final two-layout run; changing the ledger now would be a false closure.
- **P1:** `REL-001` has meaningful component evidence but no accepted release
  bundle, race result, controlled performance baseline, or long soak. The
  existing short soak must not be reported as the missing release qualification.
- **P1:** `BKP-001` cannot be promoted from in-repository restore tests because
  the ledger explicitly requires packaged clean-machine, installer lifecycle,
  and attestation evidence.
- **P1:** the baseline DB-002 row was initially classified as machine-only.
  Post-baseline fixed-seed WAL/BUSY evidence identified the additional
  repository-owned retryability gap recorded above; the CSV now classifies the
  row as repository-and-external while preserving `IMPLEMENTED`.

The absence of a new implementation is the outcome of the audit: the next
honest work is integration rerun or externally provisioned qualification, not
more production machinery.
