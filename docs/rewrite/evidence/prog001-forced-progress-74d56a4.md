# PROG-001 forced-progress recovery on `74d56a4`

Status: **IMPLEMENTED — browser qualification remains BLOCKED**

Baseline: `74d56a436e14991b44a091eafbd8e20514244649`

This slice closes the process/HTTP seam without a browser and without adding a
production test hook. It does not change ingestion, retrieval, RAG, provider,
OpenAPI, or persistence behavior.

## New process-level evidence

`v2/cmd/mindweaver/progress_recovery_test.go` invokes the production
`run(..., "serve", ...)` composition in a child process, exchanges the real
one-use bootstrap token, and uses the authenticated loopback HTTP API.

`TestServeProcessForcedAskRestartPollsDurableFailureWithoutProviderReplay`
proves the following sequence:

1. A source is uploaded and its ingestion job reaches durable success through
   the production HTTP and worker path.
2. A concrete loopback Ollama configuration is persisted. The fake server then
   blocks only the production `/api/chat` request.
3. After that provider request has arrived, exact Ask replay and answer polling
   both return HTTP 202 with durable `pending` state and never report
   `completed`. A different request cannot be inferred from an in-memory UI
   state because every observation is an HTTP response from the serve process.
4. The process is terminated with `Process.Kill`, not graceful shutdown. The
   in-flight HTTP request cannot return success.
5. A new serve process opens the same Vault. Startup diagnostics report exactly
   one reconciled pending answer; answer GET returns the same answer identity as
   durable `failed/OUTCOME_UNCERTAIN`.
6. Exact replay after restart returns that same terminal record and the fake
   provider call count remains exactly one.

## Reused ingestion evidence and deduplication decision

`v2/qualification/knowledge/doc001_remaining_qualification_test.go` already
contains `TestDOC001ForcedTerminationRecovery`. Its child process is killed
after a durable running-job claim. Startup recovery returns the job to queued
with `PROCESS_INTERRUPTED`; the next fenced attempt succeeds; exact upload
replay preserves identity; document, blob, chunk, and FTS cardinality remain
one logical ingestion; and repeated startup recovery is a no-op.

The ordinary authenticated HTTP projection is separately covered by the app
product tests: `/api/v1/jobs` reads the durable job projection and restart
reopens the same Vault. Repeating the ingestion forced-exit matrix through the
serve command would add only timing-sensitive orchestration around those same
storage and HTTP boundaries. It is intentionally not duplicated.

Together, the pre-existing ingestion process evidence and the new Ask
serve/HTTP evidence cover the durable backend and real-process HTTP boundary.
They do not close real-browser reconnect, focus, rendering, or browser-process
forced-exit behavior. PROG-001 therefore remains `IMPLEMENTED`, not `PASS`,
until the external browser qualification gate is available and succeeds.

## Validation

From `v2/` with the pinned Go toolchain:

```powershell
go test ./cmd/mindweaver -run '^TestServeProcessForcedAskRestartPollsDurableFailureWithoutProviderReplay$' -count=10
go test ./cmd/mindweaver ./internal/app ./internal/rag ./qualification/knowledge
go vet ./cmd/mindweaver
```

All commands passed on 2026-08-27. Full-module validation is recorded with the
commit handoff.

## Non-claims

- No browser, WebUI, signed release executable, or external Ollama instance is
  qualified.
- No automatic provider replay is introduced or authorized.
- No ingestion attempt is promised to stay at one after forced termination;
  the invariant is one durable logical result with fenced recovery and no
  duplicate document/blob/chunk/FTS effects.
- This evidence does not alter RET-003 or qualify natural-language retrieval.
