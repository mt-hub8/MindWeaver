# SEC-002 current-main revalidation

Status: **IMPLEMENTED; not PASS**

This evidence-only revalidation binds the existing Go CORE output audit to
committed main `8db2dea0d41bd757d7d5058e52a610f2315431e5`. The two commits after
`71bd8049e70e5a0c1cc16df89f48870f72cdb426` change only the acceptance
ledger and REL evidence; `v2/cmd`, `v2/internal`, and `v2/platform` have no
diff. No production source, ledger row, artifact identity, vendor file, or
build script is changed by this slice.

## Preserved provenance and current surface

The original audit remains immutable:

- `review.md`: canonical-LF SHA-256
  `70e14e937b5422e0fe5e1677fa09282c19a130204bc46c856d680ce3e7c785b1`;
- `surface.csv`: canonical-LF SHA-256
  `8b6542b538bbd69a17298b546691d3b09d392b6e6c1e83f230c039e2b1114fb1`;
- its 28 rows continue to describe the integrated output baseline
  `89be0fbbbbb583d621860531419b3dbc34bf53b3` and every recorded file still
  matches its SHA-256, byte count, line count, and `1-N` range.

The current reviewed surface is the case-insensitive union of those 28 rows
and the four rows in `surface-additions-8db2dea.csv`, for 32 unique paths.
The additions close omissions in the earlier operational-egress inventory;
they do not identify a new product leak.

| Added file | Classification | Release-evidence rule |
| --- | --- | --- |
| `v2/platform/version/version.go` | Controlled public metadata: version, commit, and build date | Allowed only when values are the frozen release metadata; it carries no caller input |
| `v2/internal/backup/backup.go` | Private data-bearing backup manifest and payload | Never copy manifest paths or payload bytes into logs, reports, or release attestations |
| `v2/internal/backup/residue.go` | Private recovery receipt/binding with a bounded destination leaf and opaque identities | Keep beside the user-selected destination only; reports may expose stable state/counts, never receipt fields |
| `v2/internal/backup/residue_identity_windows.go` | Opaque machine-local filesystem identity used by the private recovery binding | Never reproduce the identity token in logs or release evidence |

The remaining apparent output-like operations outside the 32-row surface are
internal buffers, hashes, migration checksums, or parser CMap construction.
They do not write a public channel or a durable diagnostic artifact.

## Deliberate private channels

SEC-002 does not claim that product data stores are content-free. Authenticated
API/UI success responses, the Vault, plaintext backup payload and manifest,
the PDF helper's private stdout pipe, the literal-loopback Ollama prompt, the
one-use bootstrap URL, the versioned config, and backup recovery receipts are
deliberate data-bearing channels. Their diagnostic projections remain bounded
and classified. None of these channels may be copied into a qualification
report merely because the report is local.

The production diagnostic sinks remain narrow: `mindweaver` writes only
`apperror.PublicMessage` to stderr, `mindweaver-pdf` reports failure only by a
stable exit code, and the loopback HTTP server discards its internal error log.
Problems, backup statuses, and recovery NDJSON expose bounded stable fields,
codes, counts, and phases instead of raw causes or paths.

## Historical evidence paths are not release inputs

Several retained historical reports contain a local toolchain/worktree path or
a machine-specific volume identifier. The affected reports include
`dual-pe-production-closure.md`, `db002-bounded-wal-stress-7b75f8f.md`,
`sqlite-qualification.md`, and
`windows-registered-volume-boundary-57e293b.md`. They remain repository
provenance and must not be copied into the final release artifact, published
attestation, SBOM/NOTICE bundle, installer log bundle, or clean-machine report.

New release evidence must use stable placeholders and relative repository
paths. Its fail-closed scanner must reject local absolute paths, UNC paths,
volume GUIDs, credentials, prompts, and source content. A hash may identify a
private input, but the report must not retain that input or its locator.

## Verification and remaining boundary

Run from the repository root:

```powershell
& .\docs\rewrite\evidence\sec002-go-output-audit\verify-current.ps1
```

The verifier checks the immutable original evidence, exact row counts and
path sets, all current file hashes/sizes/line ranges, the four addition
classifications, and the unchanged `SEC-002 = IMPLEMENTED` ledger state.

Focused canary/redaction tests, the eight security-sensitive package tests,
and their vet set pass with Go 1.27.0 in vendor/offline mode. This revalidates
the source/runtime boundary only. The existing dual-PE gate also passes without
an identity change: `mindweaver.exe` has 78 source files at
`6f6a36c470e50e902fa1e836f429bda6d7a06fcf729a2964181f496d76874f5e`,
`mindweaver-pdf.exe` has five at
`0bec9ddde1ea8778ffc3c20740ed55080d7ef0b537cfd070a2d563ccc5a087c6`,
and their 82-file union is
`6ec0fb3c1c7f4c8eefc33edaeef781c1af7a321cf8eb548ef1d0acbedf004739`.

SEC-002 remains `IMPLEMENTED` until a final
release-wide bounded scan covers the final PE and installer stdout/stderr, OS
event and crash artifacts, signed manifests, SBOM/NOTICE, browser traces or
screenshots, and published qualification reports. Passing ordinary CI and
standalone extraction does not replace those missing artifacts.
