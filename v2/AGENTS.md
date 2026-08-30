# MindWeaver Go V2 Guide

These instructions apply to the `v2/` Go module and supplement the repository
root `AGENTS.md`.

## Module Facts

- Run Go commands from `v2/` unless a script explicitly changes directory.
- Use the frozen Go 1.27.0 Windows/amd64 toolchain with `CGO_ENABLED=0`.
  `go.mod` intentionally keeps the Go 1.26 language/module directive.
- Dependencies are defined by `go.mod` and `go.sum`. There is no vendored module
  tree. Use `go mod download`, `go mod verify`, and `-mod=readonly`; do not
  reintroduce `vendor/` or offline-vendor gates without an explicit decision.
  Do not run `go get` or `go mod tidy` unless dependency declarations are an
  explicit part of the task.
- The production command set is exactly `cmd/mindweaver` and
  `cmd/mindweaver-pdf`. Do not add another executable implicitly.
- `README.md`, `openapi/v1/`, `../docs/rewrite/acceptance-ledger.md`, and the
  accepted ADRs under `../docs/rewrite/adr/` define the current product and
  contract boundary.

## Architecture Invariants

- SQLite is authoritative for current product state. Immutable blobs are
  authoritative for ingested source bytes. Chunks and FTS are rebuildable
  derivatives.
- Keep external I/O outside SQL transactions. Durable jobs must preserve lease
  ownership, fencing, cancellation, bounded retry, terminal state, and restart
  recovery.
- Publish durable authority before interrupting active work. Cancellation must
  target only the matching job, reap helper descendants, and remain stable
  after reopen.
- Keep the product loopback-only and fail closed on authority, path, identity,
  framing, size, and outcome uncertainty.
- Search and Ask use the same scoped, literal continuous-source-phrase boundary.
  Citations prove source membership and structure, not semantic truth.
- Preserve idempotency across HTTP admission, durable state, retries, process
  interruption, and exact replay. Never retry an outcome-uncertain provider
  call automatically.
- User-visible errors and evidence must be bounded and content-free. Do not emit
  prompts, answers, source text, credentials, URLs, or absolute paths.
- Backup creation is no-overwrite. Recovery verification/restore is a
  startup-only mode and must not open or fake an active product Vault.

## Scope Discipline

- Current CORE excludes legacy migration and the legacy Agent, Memory, Vector,
  Hybrid/RRF, Evaluation, Batch, natural-question semantic retrieval, OCR, and
  cloud-provider implementations. Do not resurrect them from Java.
- A user-authorized new capability must be implemented from zero as a bounded
  vertical slice and must not weaken the current Vault, lifecycle, job,
  provenance, security, or deletion invariants.
- Do not confuse a local-first runtime with an offline build. Build-time module
  download is allowed; runtime product traffic remains within its documented
  loopback boundary.
- Keep platform-specific behavior explicit. Windows 10/11 x64 is the current
  qualified product target; unsupported mutation paths must fail closed.

## Code and Contract Changes

- Format touched Go files with the `gofmt.exe` beside the frozen toolchain.
- Add or update tests with every behavior change. Prefer concrete production
  components over mocks for persistence, cancellation, restart, process, and
  idempotency boundaries.
- If routes, schemas, commands, production packages, embedded UI, migrations,
  or executable reachability change, update the corresponding OpenAPI/core
  surface contract and its tests in the same scoped change.
- Source/PE identity failures are review signals. Confirm the production source
  change is intended before updating an identity; never blindly rehash to make
  a gate green.
- Preserve the checksums and upgrade compatibility of applied migrations
  `001` through `007`. Never rewrite an applied migration; append a new one and
  add compatibility, rollback, and interruption evidence when schema changes.
- Avoid new dependencies unless the task requires them. When dependency input
  changes, update both `go.mod` and `go.sum`, run module verification, and
  document the reason.

## Validation

Set `MW_GO` to the full path of the frozen Go 1.27.0 executable. Start with the
packages touched:

```powershell
$go = $env:MW_GO
& $go test ./internal/app -count=1  # replace with the affected package(s)
& $go vet ./internal/app            # replace with the affected package(s)
```

For production, public-contract, persistence, process, or cross-package
changes, run the full gates serially from `v2/`:

```powershell
$go = $env:MW_GO
& $go test ./... -count=1
& $go vet ./...
.\scripts\ci.ps1 -Go $go
.\scripts\verify-standalone.ps1 -Go $go
```

- When production sources, routes, command reachability, or packaging inputs
  change, also run
  `& $go test ./openapi/v1 ./qualification/production -count=1`.
- Run `..\docs\rewrite\scripts\validate-legacy-inventory.ps1` when legacy
  disposition, acceptance IDs, or rewrite inventory changes.
- Run browser self-tests only when the browser qualification surface changes;
  a controlled harness or BLOCKED result is not real-browser product evidence.
- Use synthetic Vaults and literal-loopback fakes in tests. Do not connect a
  test to a real user Vault, real model, or external service unless the task
  explicitly requires and authorizes that integration.
- Do not run `ci.ps1` and `verify-standalone.ps1` concurrently.
- `verify-standalone.ps1` intentionally requires a clean committed tree. If the
  current work is uncommitted, report that gate as not yet runnable; never
  commit or discard work merely to satisfy the precondition.
- Report exact commands and outcomes. If a full gate is blocked by a known
  baseline failure, reproduce it on the baseline before attributing it to the
  current change.
