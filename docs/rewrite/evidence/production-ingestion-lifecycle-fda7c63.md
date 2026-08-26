# Production ingestion lifecycle qualification after `fda7c63`

Status: **PARTIAL PASS; DOC-001 remains NOT_IMPLEMENTED**

Parent commit: `fda7c634f6fa1698496d0589a2c48771ec03c6ef`

Executable qualification:
`v2/qualification/knowledge/production_ingestion_lifecycle_test.go`

Qualification source SHA-256:
`9b2e2a5f01e7ad5ea984a2ca025d20cae0ca0d288f4413dccf380c05cba15f5a`

## Production path exercised

The qualification opens a fresh Vault with `vault.Open`, then opens its real
Blob and SQLite stores. It builds `cmd/mindweaver-pdf` from the current source,
binds it through `pdfextract/client.New`, requires a successful production
capability Probe, and supplies that client to `workbench.NewWithPDF`.

All state changes and reads use exported production methods: `Upload`,
`CreateCollection`, `AddDocumentToCollection`, `ClaimOne`, `RunOne`,
`CancelJob`, `GetJob`, `GetDocument`, `Search`, `SearchCollection`, and
`RecoverInterruptedAtStartup`. The qualification contains no private SQL,
schema, alternate index, tokenizer, or retrieval implementation.

## Observed controls

- Regular TXT, Markdown, and generated single-page text-PDF sources each move
  from a durable upload identity to a succeeded job and active revision.
- Production global and exact-collection search returns a chunk carrying the
  same document and revision identities. The empty collection returns no hit.
- Exact upload replay returns the original document/revision/job/blob identity
  without creating another document; a changed payload under that identity is
  rejected as an idempotency conflict. The exact replay remains stable after
  reopen.
- A queued cancellation remains cancelled, has no active revision, and leaks
  no global or scoped FTS result.
- A malformed PDF converges to failed/`PDF_INVALID`, has no active revision,
  and leaks no global or scoped FTS result.
- One claimed job is closed with a still-live lease. Reopening the same Vault
  and calling startup recovery returns exactly that job to queued with
  `PROCESS_INTERRUPTED`; the production worker convenience then succeeds on
  attempt two.
- A second close/reopen preserves the exact Vault paths, collection IDs,
  document IDs, revision IDs, job IDs, terminal states, memberships, chunks,
  and FTS results. Repeating startup recovery changes zero jobs.

No new production defect was observed in these controls.

## Deliberate limitations

- Closing the stores after a committed running claim is a restart-recovery
  shape, not an operating-system forced-kill test and not proof for every
  persistence boundary.
- Inputs are small regular controls. This slice does not qualify oversize,
  timeout, memory, disk-full, hostile-PDF, or maximum-chunk boundaries.
- It composes the production services directly; it does not qualify multipart
  HTTP authentication, the App worker goroutine, or a packaged release helper.
- It supplies no latency, capacity, or strict bounded-work conclusion.
- It does not select relational, hybrid, query-expansion, embedding, vector,
  rerank, or query-understanding behaviour and changes no production code,
  schema, OpenAPI, WebUI, backup, release, or migration.

DOC-001 therefore remains open for the forced-exit and bounded-input matrix;
JOB-002 is not closed by this single recovery control.

## Validation on 2026-08-27

- New focused test: PASS at `-count=1` and `-count=10`.
- Entire `qualification/knowledge`: PASS at `-count=10`.
- `internal/blob`, `internal/pdfextract/client`,
  `internal/pdfextract/parser`, `internal/store/sqlite`, `internal/vault`, and
  `internal/workbench`: PASS together at `-count=10`.
- Full-module `go vet ./...`: PASS.
- Full-module `go test ./...`: RUN, NOT PASS. All listed production and
  qualification packages passed except the pre-existing
  `openapi/v1.TestEmbeddedContractMatchesProduction` manifest mismatch
  (`7de68fba33368f4cfa74a44cb27a0c5add95c688f70c0c77b939bbb21d825dca`
  actual versus
  `325dfec05db7f7744c6aaf4de792d80efaa6b458a0b850a635d3c41e3596e099`
  contracted). The identical failure was already reproduced at clean exact
  baseline `d61a158`; this slice does not modify OpenAPI.
