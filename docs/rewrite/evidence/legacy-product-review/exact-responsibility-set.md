# Legacy product exact responsibility set

## Frozen scope

- Baseline: `0df22ddaf02c64bf73a7df12cd5fea6b52632c73`
- Machine-readable manifest: [`exact-responsibility-set.csv`](./exact-responsibility-set.csv)
- Verification: `pwsh -NoProfile -File docs/rewrite/evidence/legacy-product-review/verify.ps1 -Manifest docs/rewrite/evidence/legacy-product-review/exact-responsibility-set.csv`

The manifest is generated from Git tree identity, not package-name search. It
contains every blob in the nine required main and test Java package trees
(`rag`, `llm`, `modelprovider`, `agent`, `memory`, `evaluation`, `batch`,
`kbhealth`, and `grounding`), the complete `src/test/resources/evaluation`
tree, and the complete `src/main/resources/static` tree.

| Exact surface | Files | Lines | Decision |
| --- | ---: | ---: | --- |
| Required main Java packages | 131 | 9,178 | `DROP` |
| Corresponding test Java packages | 43 | 4,607 | `DROP` |
| Evaluation test resources | 2 | 57 | `DROP` |
| Legacy static UI | 47 | 10,223 | `DROP` |
| **Unique total** | **223** | **24,065** | **zero exceptions** |

The 176 non-static files occur exactly once across the three detailed review
manifests: [`rag-provider.csv`](./rag-provider.csv),
[`agent-memory.csv`](./agent-memory.csv), and
[`evaluation-batch-kbhealth.csv`](./evaluation-batch-kbhealth.csv). Verification
fails if any direct file is absent or appears in more than one of those slices.

The eight static files cited by earlier partition reviews are dependency
evidence only. Static ownership and repository-wide counts come exclusively
from [`legacy-static-ui.csv`](./legacy-static-ui.csv), which covers all 47
static files exactly once. Repeated references therefore do not inflate the
24,065-line total.

## Disposition

Every Java implementation, test, evaluation fixture, and static asset in this
exact set is `DROP`. There are no class, method, prompt, DTO, endpoint, page,
CSS, script, fixture, or schema exceptions. The detailed reports record why
weak citation heuristics, mutable provider/evaluation paths, speculative Agent
and Memory layers, non-atomic Batch operations, health claims, and the legacy
browser surface are not implementation salvage candidates.

Product ideas that remain useful are independently implemented or hardened in
the current Go CORE. This decision does not authorize copying Java code and
does not restore the abandoned Java/MySQL export, neutral package, importer,
`mindweaver-migrate`, or SQLite 007 work.

## Coverage result

The verifier checks each row against the immutable baseline Git blob, recomputes
SHA-256 and complete `1..line_end` coverage, re-enumerates the exact directory
trees, requires one non-empty owner and `DROP` decision per row, and asserts the
223-file / 24,065-line totals. This is the unique master for the delegated
responsibility set.
