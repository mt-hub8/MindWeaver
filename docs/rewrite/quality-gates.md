# Rewrite quality gates

## Gate 0: baseline and scope

- Every legacy endpoint, page, persistent table, background task, provider,
  configuration, and user-owned data class has a disposition.
- Known Java defects are marked as non-compatible behavior.
- Golden fixtures are synthetic or explicitly sanitized.

## Gate 1: technical feasibility

- SQLite driver/CGO, WAL, backup, job claim, disk-full, and crash behavior measured.
- One production keyword-retrieval boundary is explicitly selected and passes
  versioned minimum recall, scope-isolation, capacity, and latency thresholds;
  rejected semantic candidates stay absent.
- PDF parsing compared with existing corpus and isolated from the main process.
- Loopback Ollama timeout, cancellation, malformed responses, and bounded output verified.
- Windows locking, sleep/resume, signed MSI install, upgrade, rollback, and
  uninstall verified. No credential store or built-in updater is implied.

## Gate 2: standalone walking skeleton

- `v2/` builds and tests after copying to an empty directory.
- A clean Windows environment can open, lock, migrate, recover, serve, and close a Vault.
- Browser bootstrap/session security and API version negotiation pass.
- Logs contain no secret, prompt, source text, or persistent bootstrap token.

## Gate 3: data safety kernel

- Blob intents, durable jobs, fencing, reconciliation, backup/restore, purge planning,
  low-disk mode, and fault injection pass.
- A forced termination after every persistence checkpoint produces either a valid
  state or an explicit recoverable `NEEDS_ATTENTION` state.

## Gate 4: knowledge lifecycle

- TXT, Markdown, and supported PDF upload flows close success, failure, retry,
  cancellation, crash, trash, restore, and purge paths.
- No old, failed, trashed, purging, or foreign-scope generation is retrievable.
- Reindex remains absent until a later accepted slice proves its isolation.

## Gate 5: RAG correctness

- Search and Ask use the same scoped production keyword query.
- Empty scope remains empty and scope leak is zero in the certified corpus.
- Strict citations all belong to exact AnswerContext items.
- Refusal changes the final answer.
- Provider timeout/cancel/outcome-uncertain states remain consistent and diagnosable.

## Gate 6: product, security, and operability

- First-run, no-model, offline, low-disk, recovery, backup, restore, uninstall, and
  verified purge workflows pass on a clean Windows machine.
- Localhost CSRF/DNS rebinding/XSS, provider SSRF, credential leakage, malicious
  document, path traversal, and oversized response tests pass.
- Keyboard, focus, high-contrast, scaling, and Chinese input acceptance pass.

## Gate 7: fresh-Vault adoption and Go upgrades

- A clean installation creates and reopens a new Go Vault, then ordinary
  TXT/Markdown/PDF upload builds the only supported initial dataset.
- Every declared supported Go SQLite schema version upgrades through the normal
  application path with backup, rollback/failure, and compatibility evidence.
- Command, dependency, and package audits prove that no Java/MySQL exporter,
  neutral legacy package, legacy importer, or migration executable is shipped.

## Gate 8: release qualification

- Race, fuzz, fault injection, E2E, performance, soak, N-1/N-2 upgrade, backup
  restore, offline egress, signing, SBOM, vulnerability, and license gates pass.
- No unaccepted P0/P1 data, security, upgrade, or release risk remains.

## Gate 9: fresh-Vault release and repository extraction

- Java is frozen historical reference and Go is the only product writer.
- A clean first-run record proves the fresh-Vault path; no legacy data exit is
  advertised or packaged.
- The extracted Go repository builds and packages from a clean clone without any
  parent-directory or Java dependency.
- The legacy repository points users to the new product and explicitly states
  that supported source files must be uploaded into a fresh Vault.

## Per-capability closure matrix

Each retained capability must have evidence for:

```text
user goal
UI/API contract
domain invariant
state transition
database and file writes
external calls
success
validation failure
duplicate
concurrency
cancel/timeout
forced termination/restart
sleep/low disk
security/privacy
diagnostics
backup/upgrade
Go schema upgrade/rollback
automated acceptance test
user documentation
```
