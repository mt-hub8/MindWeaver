# DOC-001 final qualification on `bb176c8`

DOC-001 verdict: **PASS**

Repository integration verdict: **BLOCKED by an unrelated clean-checkout EOL
P0 described below**

Baseline: `bb176c8a2bd7a57558b52055527f49765e88105e`

Qualification commit: `fc667be820692e5d337ac5b1a0a88e39a1d02ae0`

Executable evidence:

- `v2/qualification/knowledge/production_ingestion_lifecycle_test.go`
  (`abca7770fb56475065dbe8781665df8c5dd9e288196d6d3a669eed43b84fce8c`)
- `v2/qualification/knowledge/doc001_remaining_qualification_test.go`
  (`8fe6a60f00922f6e91b5b2b40b2a9f9765a0b502e3d51530d10fbd42bfe191f1`)

## Qualified production behaviour

The two qualification files compose a fresh production Vault, Blob store,
SQLite store, workbench and worker path. The PDF controls build the current
`cmd/mindweaver-pdf`, require its production capability Probe and parse through
the production isolated client. The qualification uses no private SQL or test
repository implementation.

- TXT, Markdown and text-PDF are accepted at the effective workbench source
  ceiling of exactly 4 MiB. Each is ingested, activated and returned exactly
  once by global and exact-collection FTS with the accepted document,
  revision and chunk identities.
- Each format at 4 MiB plus one byte is rejected as `blob.ErrTooLarge` before
  a durable document graph is created.
- Exact upload replay at the boundary preserves document, revision, job and
  content-addressed Blob identities and does not add a document, referenced
  Blob, chunk or FTS hit.
- The public durable observations agree after replay: `ListDocuments` and
  `ReferencedBlobIDs` equal the exact expected identity sets; every referenced
  Blob opens successfully; `CleanupStaging` removes zero residue; each unique
  successful query returns one chunk; `IntegrityCheck` confirms SQLite, FK and
  external-content FTS consistency.
- Invalid UTF-8 TXT and NUL-bearing Markdown converge to failed
  `SOURCE_INVALID_TEXT`; malformed PDF converges to failed `PDF_INVALID`.
  None activates or leaks through global or scoped FTS.
- Queued cancellation remains cancelled across exact replay, has no active
  revision and has zero global/scoped hits.
- A child process opens the real Vault, accepts a TXT upload and commits a
  one-hour RUNNING lease through production `ClaimOne`. Only after a synced,
  atomically published marker proves that state does the parent terminate the
  process with `Process.Kill`; no child cleanup runs.
- The next Vault owner performs production staging cleanup and
  `RecoverInterruptedAtStartup`. Exactly one job returns to queued with
  `PROCESS_INTERRUPTED`, succeeds through production `RunOne` on attempt two,
  and remains one document, one referenced Blob and one FTS chunk after exact
  replay. Repeated startup recovery changes zero rows.

Together with the already committed Blob publication/candidate fault tests,
PDF isolation/resource qualification and ordinary lifecycle/reopen controls,
this closes the bounded admission, idempotency and restart-recovery outcome
required by DOC-001. It does not claim OCR support and introduces no OCR,
embedding, vector, hybrid or query-expansion path.

## Validation on 2026-08-27

- New bounded and forced-termination tests: PASS at `-count=1` and
  `-count=10`.
- Entire `qualification/knowledge`: PASS at `-count=10`.
- Blob, ingest, PDF client/parser/protocol, SQLite, Vault and workbench package
  suites: PASS.
- Full-module `go vet ./...`: PASS.
- At the exact qualification commit in a detached worktree hydrated with
  `core.autocrlf=false`: offline `scripts/ci.ps1` PASS and
  `scripts/verify-standalone.ps1` PASS.

## Open P0: Git-clean checkout can change embedded source bytes

The requested independent worktree was created under the host's global
`core.autocrlf=true`. `v2/internal/webui/static/app.css` has no `eol`
attribute, so that Git-clean worktree contains 5,853 bytes and 77 CRLF line
endings. Exact clean `bb176c8` and the LF verification worktree contain 5,776
bytes and 77 LF line endings. Git reports no diff, but command source manifests
bind embedded files as raw bytes.

Consequently CI and standalone in the requested worktree are RUN but BLOCKED:

- mindweaver manifest actual
  `e81e897c58aa62c713da3353b52aa1a15e9ab609fe5bfcfb8bab270174c0ee59`,
  contract
  `006c3d00b663d6ab6417935c50c501c508b8e26783a9367ef46397f6969573e2`;
- dual-PE union actual
  `2296b3974ffcd33887416fbcd6b510725a06a6951d60e0faf30369ced7041889`,
  contract
  `967be95103379b1d20e4310b82cbbd9ced377bd989427f20e196a137820e7bce`.

This is a clean-checkout/release reproducibility P0, not a DOC-001 failure.
The release/WebUI contract owner must freeze LF checkout for raw embedded
assets or otherwise make the raw-byte manifest contract checkout-invariant,
then rerun both gates from an ordinary fresh Windows worktree. This slice does
not modify WebUI, OpenAPI, release metadata or repository attributes.
