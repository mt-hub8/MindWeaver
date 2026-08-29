# Legacy Java full-file audit manifest

[`legacy-file-manifest.csv`](./legacy-file-manifest.csv) is the fail-closed
work queue for reviewing every tracked legacy Java source, test, and text
resource. It is deliberately separate from the artifact-oriented
[`legacy-inventory.csv`](./legacy-inventory.csv): the artifact inventory finds
routes, tables, dependencies, and other declarations, while this manifest makes
it impossible to mistake partial declaration discovery for full-file review.

The initial manifest was generated from Git commit
`0df22ddaf02c64bf73a7df12cd5fea6b52632c73`. It contains exactly 866 files:

| Kind | Files | Git-blob bytes | Physical lines | Nonblank lines |
| --- | ---: | ---: | ---: | ---: |
| `MAIN_JAVA` | 587 | 1,495,516 | 40,009 | 33,729 |
| `TEST_JAVA` | 193 | 872,451 | 21,282 | 17,887 |
| `MAIN_RESOURCE` | 84 | 436,932 | 11,154 | 10,066 |
| `TEST_RESOURCE` | 2 | 2,668 | 57 | 48 |
| **Total** | **866** | **2,807,567** | **72,502** | **61,730** |

The current physical copies are isolated under `legacy/java/`, but the CSV
`path` column deliberately retains the pre-archive logical locators (`src/...`).
The validator reads `legacy/java/<path>`, strips that physical prefix, and then
compares the same immutable blob identities. A directory move therefore does
not masquerade as a source review or reset historical provenance.

All 866 initial rows are `STRUCTURAL_SCAN_ONLY`. That level proves only the
tracked path, Git blob identity, SHA-256, strict UTF-8 decoding, byte and line
counts, Java package/path agreement, and structural partition. It is not a
claim that a human has read the file or that the implementation is correct.

## Binding disposition policy

Every row has `implementation_disposition=DROP`, including rows that may later
yield useful requirements or business semantics. This encodes the current
architecture decision that no Java implementation is authorized for reuse.
A human full-file review may populate `feature_ids`, `semantic_candidates`,
risks, and evidence, but it cannot promote the implementation by editing one
CSV row. Changing the only legal implementation disposition requires a prior
architecture-policy and validator change.

This distinction is intentional:

- a useful user need can be rebuilt in Go without preserving its Java design;
- a semantic candidate is evidence for requirements analysis, not code reuse;
- tests and resources are reviewed for hidden behavior and risk, not treated as
  proof that the corresponding production implementation is closed-loop;
- `STRUCTURAL_SCAN_ONLY` and `UNREVIEWED` never count as line-by-line review.

## Schema

| Column | Meaning |
| --- | --- |
| `path` | Canonical pre-archive logical path; the current physical file is `legacy/java/<path>`. |
| `git_blob` | Exact Git blob object ID read from `HEAD`. |
| `source_sha256` | Lowercase SHA-256 of the raw Git blob bytes, not checkout bytes. |
| `bytes` | Raw Git blob byte count. |
| `physical_lines` | Cross-platform count over CRLF, LF, or CR terminators in the raw UTF-8 blob. |
| `nonblank_lines` | Physical lines containing non-whitespace text. |
| `kind` | One of the four frozen source roots. |
| `package` | Unique declared Java package; empty for resources. |
| `partition` | Structural package/resource partition, not a semantic conclusion. |
| `feature_ids` | Ordinal, unique `;`-separated feature candidates. Empty until human review. |
| `review_level` | `UNREVIEWED`, `STRUCTURAL_SCAN_ONLY`, `RANGE_REVIEWED`, or `FULL_FILE_REVIEW`. |
| `review_ranges` | Ordered `start-end` ranges. Adjacent ranges may record review batches but cannot leave holes inside the claimed interval. Full review must cover `1..physical_lines`. |
| `implementation_disposition` | Fixed to `DROP` by current policy. |
| `semantic_candidates` | Stable, content-free semantic identifiers found by human review. |
| `risk_ids` | Closed risk identifiers enforced by the validator. |
| `evidence` | Stable evidence locators; structural-only rows use `STRUCTURAL_METADATA_ONLY`. |
| `reviewed_commit` | Full Git commit containing the reviewed blob. |
| `reviewed_sha256` | SHA-256 that must equal the current source SHA-256. |
| `reviewer` | Stable reviewer identifier. |
| `notes` | Bounded single-line rationale; required for human review. |

Human-reviewed rows must bind `reviewed_commit:path` to the same `git_blob` and
bind `reviewed_sha256` to `source_sha256`. A source identity change causes the
generator to reset the row to `UNREVIEWED`, clear all human review claims, and
retain only the safe `DROP` default. Unchanged path/blob/SHA triples preserve
review metadata.

## Validation and regeneration

Run from the repository root with PowerShell 7:

```powershell
pwsh -NoLogo -NoProfile -File docs/rewrite/scripts/validate-legacy-file-manifest.ps1 -SelfTest
```

Regenerate structural metadata after an intentional committed legacy-source
change:

```powershell
pwsh -NoLogo -NoProfile -File docs/rewrite/scripts/validate-legacy-file-manifest.ps1 -EmitManifest -SelfTest
```

The validator reads source blobs through one bounded `git cat-file --batch`
session. Consequently `core.autocrlf`, checkout encoding, and platform newline
translation cannot change the recorded source SHA-256, bytes, or line counts.
The manifest is strict UTF-8 without BOM and has one canonical logical CSV
representation; it accepts either LF or uniformly Git-normalized CRLF in the
checkout, then compares the normalized logical bytes. It also requires the four
covered physical roots below `legacy/java/` to be clean before validating.

Validation fails on any exact-set/count drift, stale structural hash, malformed
or noncanonical CSV, UTF-8 BOM or invalid UTF-8, duplicate or case-colliding
paths, reordered rows/tokens, unknown columns/enums/IDs, unbounded fields,
mixed or lone-CR line endings, gaps in claimed review coverage, or review
commit/blob/hash mismatch.
