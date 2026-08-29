# CORE keyword-search boundary on `d61a158`

> Historical evidence. ADR 0012, the current acceptance ledger, and
> `core-keyword-boundary.8aadf60.v2.json` supersede this file's CORE/LATER
> product classification. This file's `Selection: NONE` and all candidate
> limitations remain binding; it does not authorize a retrieval implementation.

Status: **RET-003 BLOCKED**

Selection: **NONE**

Product boundary: `CORE_KEYWORD_SEARCH_ONLY_NATURAL_QUESTION_EXPLICITLY_LIMITED`

This evidence binds the minimum retained keyword-search behaviour to committed
baseline `d61a158afdc7c86914c70a847ce4e9d6fe686517`. It does not select or propose a
new retrieval implementation.

## Reproducible inputs

- Contract/corpus:
  `v2/testdata/qualification/knowledge/core-keyword-boundary.d61a.v1.json`
- Corpus SHA-256:
  `3e81a3ed29077cac9bb5f6be3d6e81bb9bd757799f0e8233544535f16a58e132`
- Executable qualification:
  `v2/qualification/knowledge/core_keyword_boundary_test.go`
- Qualification source SHA-256:
  `e8b5147ac73ac35053236b1911b122e2909645feeb0279b77f1c2bb52d6928c8`

The qualification builds documents and collections through the production
`workbench.Service`, runs the production ingestion worker, transitions the
trash control through the production lifecycle service, and calls the
production `Search` or `SearchCollection` method for every probe. It contains
no alternate SQL, tokenizer, index, ranking, schema, or migration.

## What the fixture proves

- A continuous source phrase of at least three runes (`潮汐锁定`) is found by
  the current exact-phrase trigram path.
- Global search sees both active source-positive controls; Alpha and Beta
  collection scopes return only their explicit member; the empty collection
  returns no result and does not widen globally.
- A unique term is retrievable before trash, then excluded after the production
  trash transition.
- A queued document with no active revision is excluded. The existing
  `TestSearchPlainTextActivationScopeAndCascadeCleanup` store regression also
  verifies that an indexed but inactive revision is excluded by the active
  revision predicate.
- A two-rune CJK query (`潮汐`) returns `ErrQueryTooShort`; it never falls back
  to a whole-corpus substring scan.
- A natural question whose fact anchors occur in an active document but whose
  full sentence does not occur there returns no result. This is a disclosed
  product limitation, not a recall success.

## What this fixture does not prove

- It is not a representative Chinese lexical quality corpus and supplies no
  recall, precision, FDR, latency, disk, capacity, or SLO conclusion.
- Observing small fixed fixture work is not evidence that production query work
  is strictly bounded at representative catalog sizes.
- It does not qualify natural-language question answering, query expansion,
  reranking, embeddings, vectors, hybrid retrieval, or a relational term
  index. No previously failed candidate is retained.
- It does not authorize DDL, a migration/008, or a production retrieval change.

RET-003 therefore remains BLOCKED. The retained CORE is explicit keyword
search with exact lifecycle and collection scoping; natural questions remain
explicitly limited.

## Validation on 2026-08-27

- Focused qualification and inactive-revision regression, each at `-count=10`:
  PASS.
- `qualification/knowledge`, `internal/store/sqlite`, `internal/workbench`,
  `internal/lifecycle`, and `internal/rag`, together at `-count=10`: PASS.
- Relevant-package vet and full-module `go vet ./...`: PASS.
- `scripts/ci.ps1` and `scripts/verify-standalone.ps1`: RUN, NOT PASS. Every
  listed package, including `qualification/knowledge`, passed before the
  pre-existing `openapi/v1.TestEmbeddedContractMatchesProduction` failure.
  That failure reports command source manifest
  `7de68fba33368f4cfa74a44cb27a0c5add95c688f70c0c77b939bbb21d825dca`
  while the baseline contract contains
  `325dfec05db7f7744c6aaf4de792d80efaa6b458a0b850a635d3c41e3596e099`.
  The identical failure was separately reproduced in a clean detached worktree
  at exact baseline `d61a158`. This qualification does not modify OpenAPI or
  claim the full gate is green.

## Reproduction

From `v2/`, with the pinned Go toolchain and offline module settings:

```powershell
go test ./qualification/knowledge -run '^TestCoreKeywordBoundary' -count=10
go test ./internal/store/sqlite -run '^TestSearchPlainTextActivationScopeAndCascadeCleanup$' -count=10
go vet ./qualification/knowledge ./internal/store/sqlite
```
