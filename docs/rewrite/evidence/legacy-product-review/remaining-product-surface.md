# Remaining legacy product surface review

## Frozen residual set

- Baseline: `0df22ddaf02c64bf73a7df12cd5fea6b52632c73`
- Machine-readable manifest: [`remaining-product-surface.csv`](./remaining-product-surface.csv)
- Verification: `pwsh -NoProfile -File docs/rewrite/evidence/legacy-product-review/verify.ps1 -Manifest docs/rewrite/evidence/legacy-product-review/remaining-product-surface.csv`

The residual set is an exact subtraction. The full legacy runtime/build tree is
every baseline blob below `src/main/java`, `src/test/java`,
`src/main/resources`, `src/test/resources`, `workers`, and `scripts/windows`,
plus the Maven wrapper/build and two Compose entry points. Removing the 223-file
primary responsibility set leaves these 667 unique files:

| Residual surface | Files | Lines | Decision |
| --- | ---: | ---: | --- |
| Maven / Compose build entry points | 6 | 682 | `DROP` |
| Remaining main Java | 456 | 30,831 | `DROP` |
| Remaining resources | 37 | 931 | `DROP` |
| Windows scripts | 4 | 318 | `DROP` |
| Remaining Java tests/resources | 150 | 16,675 | `DROP` |
| Python workers and generated worker files | 14 | 2,014 | `DROP` |
| **Residual total** | **667** | **51,451** | **zero exceptions** |

The exact and residual manifests are disjoint. Their union is the complete
frozen legacy runtime/build tree: **890 unique files / 75,516 lines**. The
verifier reconstructs that union from the baseline Git tree and fails on any
missing, extra, overlapping, or reassigned path.

## Product decision

All residual Java controllers, services, repositories, entities, DTOs,
retrieval/vector/embedding experiments, MySQL migrations, configuration,
tests, workers, launch scripts, and build entry points are implementation
`DROP`. This is a deletion decision, not an instruction to recreate feature
parity in Go.

The current Go first-release CORE keeps its independently designed document
lifecycle, TXT/Markdown/PDF ingestion, collections, search, scoped Ask,
credential-free loopback Ollama, conversation history, and Backup/Restore.
Normal Go schema migrations and future Go-to-Go upgrades are outside this
legacy-drop decision. Agent, Memory, Evaluation, Batch, Notification, Qdrant,
rerank, hybrid/query experiments, Java-derived data, and Java vector state stay
deferred or dropped and must start from a new product need if reconsidered.

The abandoned Java/MySQL migration chain remains entirely dropped: no exporter,
neutral-v1 package, Go legacy importer, `mindweaver-migrate`, or SQLite 007 is
revived by this review.

## Risk summary

Detailed partition reports retain the concrete P0/P1 evidence. The residual
manifest closes the repository-level omission risk: no unreviewed legacy Java,
resource, worker, script, or executable build entry remains hidden outside the
named package slices. It does not claim that legacy code is safe to run, that a
real MySQL source was qualified, or that the legacy browser/worker products
have a closed security boundary.
