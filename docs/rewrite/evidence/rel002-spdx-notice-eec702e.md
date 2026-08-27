# REL-002 single-SPDX qualification rewrite

Status: **qualification evidence only; REL-002 remains `NOT_IMPLEMENTED`**

- Production baseline: `e8cf45db8482504423c23e17a013f2ba1e2f1dfa`
- Qualification code commit: `eec702e6f4f25fbc6d6feae935d4afb3c5d17b90`
- Branch: `codex/rel002-supplychain-e8cf45d`
- Toolchain exercised: `go1.27.0 windows/amd64`
- Scope: supply-chain qualification tests, pinned official schema input,
  committed qualification documents, and this evidence. No production,
  ledger, release identity, installer, signature, or SQLite provenance file
  changed.

## Deletion-first result

No repository consumer was found that requires both SPDX and CycloneDX. This
rewrite keeps one canonical inventory, **SPDX 2.3**, NOTICE, and a hash
manifest. CycloneDX, its renderer, its document, and its three referenced JSON
schemas are dropped.

The rejected chain ending at `14c40fc` added 10,875 lines in 16 files. This
replacement code commit adds 3,427 lines in 10 files. Of those, 832 lines are
the byte-for-byte official SPDX schema and repository license and 1,160 lines
are committed qualification outputs. The ordinary tests are read-only; the
tree contains no update environment variable, fixture writer, or generator.

There is also no safe regeneration command. Providing one without extracting
the real PE build, buildinfo, legal evidence, and render contracts into one
shared qualification package would create a second inventory truth source.
That refactor is not justified in this slice. The machine inventory therefore
contains `FIXTURE_REGENERATION_NOT_IMPLEMENTED`; future source/PE changes
require a separate reviewed implementation before fixtures can be claimed
maintainable. Temporary test modification or hand-editing generated JSON is
not an accepted maintenance workflow.

## Qualification inputs and honest attestation boundary

The inventory records these reviewed qualification inputs:

- repository `github.com/mt-hub8/MindWeaver`;
- revision `e8cf45db8482504423c23e17a013f2ba1e2f1dfa`;
- `v2` tree `git-sha1:abdc2a4b7c4932f441beff3dfd20b1d634f3d722`;
- reviewed production source-union SHA-256
  `079e4f0a337b4a2d7df2ccf4e42e6f48132018bfe1ef269a18158b09558b4592`;
- qualification product version `0.0.0-qualification.e8cf45d`;
- qualification `SOURCE_DATE_EPOCH=1787859296`, rendered as
  `2026-08-27T19:34:56Z`;
- qualification renderer identity
  `MindWeaver supply-chain qualification-1.0.0`.

The product version is explicit and cannot silently become `(devel)`. The
renderer version is a separate SPDX Tool creator version. The timestamp is a
qualification fixture, not release publication time. Revision, tree, and
source-union are reviewed inputs and are not independently derived from Git
inside standalone validation. Therefore this is not a source-to-PE
attestation.

Machine fields are deliberately named `qualificationBlockers`,
`Supply-Chain-Qualification-Blockers`, and “Pre-package supply-chain
qualification blocked”. They are not represented as an exhaustive REL-002
release blocker list. Their exact values are:

- `FIXTURE_REGENERATION_NOT_IMPLEMENTED`;
- `PROJECT_LICENSE_MISSING`;
- `RELEASE_METADATA_MISSING`;
- `SOURCE_REVISION_ATTESTATION_MISSING`;
- `VULNERABILITY_EVIDENCE_MISSING`.

## SPDX semantic model

All SPDX product, artifact, and component packages use
`filesAnalyzed=false`. The SPDX document consequently contains **no SPDX File
nodes** and no component-to-evidence `CONTAINS` relationships. Product
`CONTAINS` artifact packages remains valid package containment. Component
license/upstream evidence stays authoritative in the canonical inventory and
NOTICE. Each component comment binds its inventory evidence path and raw
SHA-256 while explicitly labelling those bindings as “not analyzed package
files”.

The semantic validator requires:

- the exact product package fields and qualification source/version binding;
- exact artifact name, main package, byte size, raw SHA-256, dependency set,
  download location, `filesAnalyzed`, license fields, copyright,
  external-reference set, comment, and source binding;
- exact component ref, kind, name, version, Go h1, license expression,
  evidence refs, and dependencies against the committed module/runtime/SQLite
  qualification contracts;
- exact evidence ref, repository-relative path, kind, byte size, raw SHA-256,
  SPDX candidate and hash-matching text against those committed contracts;
- one versioned SPDX Tool creator;
- unique package IDs and relationship triples, known endpoints, and the exact
  inventory-derived relationship graph;
- the exact extracted SQLite license text.

These checks establish equality to reviewed committed qualification contracts
and freshly built artifact identities. They do not independently validate the
authority of those contracts or convert reviewed license candidates into
legal conclusions.

Mutation coverage includes duplicate JSON names, h1/raw-SHA confusion,
revision and `(devel)` changes, artifact main-package/identity/dependency
changes, duplicate dependencies, component field/license changes, evidence
path/hash changes, duplicate evidence, an unversioned Tool creator, changed
artifact SPDX fields, unknown SPDX fields, duplicate/missing relationships,
external `status=PASS`, and manifest hash changes.

## Official SPDX schema input

The offline validator accepts an exact two-file schema directory retained
byte-for-byte from `spdx/spdx-spec`, tag `v2.3`, peeled commit
`aadf3b0b8dbbabdb4d880b0fc714255fea436ff7`:

| Input | Raw SHA-256 |
| --- | --- |
| `schemas/spdx-schema.json` | `5f0df4da417edaeba5923e80431d8ac0ac2a16705710e8eb9d6587077c0b6af9` |
| repository `LICENSE` | `ddaec2160900e2dcda683a86274e1d48703dd2a3ea584397299463357b51e949` |

The script verifies the exact schema file set and hashes, then executes the
installed `Microsoft.PowerShell.Utility/Test-Json` JSON Schema engine. It
fails closed when the trusted validator is unavailable or too old. Schema
success is only structural JSON validation; the Go semantic checks above are
separate and required.

## Bound qualification documents

| File | Bytes | Raw SHA-256 |
| --- | ---: | --- |
| `inventory.json` | 29,816 | `99deb92e0df115906fe1ce0f06479573b0dfa4333b1e6de2249c57ee20ce0992` |
| `sbom.spdx.json` | 19,471 | `03b89bda348959cc3a9e0a1f1c3f39483487857add5f8913828cb7b6da0d0b27` |
| `NOTICE.txt` | 24,054 | `d0040d409cb19c4cc1e13f045362071bbeb6b8ef8bae71dc8cf9dc315b608550` |

The real frozen artifact contracts, rechecked against fresh builds, are:

- `mindweaver.exe`: 32,861,696 bytes,
  `127321290927e6816f94fafea4226d7be572d98b99d1c4d5bf17b799df7e587f`;
- `mindweaver-pdf.exe`: 8,453,632 bytes,
  `a2a6a04b4ade9367aab6cce35e9a3c87351f9c8fabd33d9ea2fe108f7e732241`.

They are qualification identities for the stated baseline, not final release
identities.

## Verification

```text
go test ./qualification/supplychain \
  -run '^(TestPrepackageSupplyChainInputClosure|TestDerivedSupplyChainDocumentMutationsFailClosed)$' \
  -count=1
PASS (19.612s)

go vet ./qualification/supplychain
PASS

pwsh -NoLogo -NoProfile -NonInteractive -File \
  qualification/supplychain/validate_official_schemas.ps1 -ModuleRoot .
PASS (exit 0)

git diff --check
PASS
```

## Remaining REL-002 blockers and non-claims

The qualification blocker list above is intentionally not exhaustive for
REL-002. Complete release closure still requires, at minimum:

- owner-approved project LICENSE and real release version/creation metadata;
- independently derived revision/tree/source-union attestation;
- bounded offline replayable vulnerability evidence;
- approved offline WiX/EULA inputs, real MSI generation, trusted signing
  material, Authenticode verification, and ICE validation;
- clean non-admin VM install/launch, N-1/N-2 upgrade, rollback, uninstall,
  retained-Vault/backup restore, and offline-egress evidence.

No P0 was found in this qualification slice after the semantic rewrite. The
fixture-regeneration gap and named release gaps remain open; this document
does not claim P1=0, release PASS, legal approval, or vulnerability PASS and
does not request a ledger change.
