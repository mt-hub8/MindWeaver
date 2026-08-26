# Runtime interruption qualification on `6c76b3d`

Status: **PASS for the two frozen interruption cases only**

- Recorded: 2026-08-27 (Asia/Shanghai)
- Tested production source commit:
  `6c76b3d25a9bb8c30baeab989e03174fe9f90833`
- Toolchain: `go version go1.27.0 windows/amd64`, `CGO_ENABLED=0`
- Module network: disabled (`GOPROXY=off`, `GOSUMDB=off`,
  `GOTOOLCHAIN=local`, `GOWORK=off`)
- Real `mindweaver.exe` SHA-256:
  `8af2fb0eb8e5c35f0e9e4474351eed9962d805ca4b1648871a549c4fd5f01329`
- Qualification-only PDF blocker SHA-256:
  `766493d8a50f09ab909dbceea63855995b6c6c70d1dbff46e7779e0a76c1f1cb`
- Machine evidence SHA-256:
  `ba1b4088933599db779b0699a8b5fc6b8fa3edd85d47225e8585227e214e46f5`

The machine report is
`v2/testdata/qualification/runtime/evidence-v1.json`. Its normal test contract
strictly decodes the report, binds the raw file hash, requires ten complete
runs for each case, checks every state and identifier-hash shape, and rejects
raw path, URL and fixture-content categories. The report contains stable codes,
counts and hashes rather than prompt, answer, source text, Vault path or
provider address.

This qualification adds no production hook. It builds the real application,
opens a fresh temporary Vault for every run, drives the real loopback HTTP API,
kills and reaps the real application process, then inspects SQLite/FTS only
after process exit. The PDF blocker is qualification-only and is never linked
into either shipped command.

## Frozen results

| Case | Checkpoint | Required final state | Runs | Result |
| --- | --- | --- | ---: | --- |
| `ANSWER_PROVIDER_RECEIVED_BEFORE_TERMINAL` | `PROVIDER_REQUEST_RECEIVED_ANSWER_PENDING` | `ANSWER_FAILED_OUTCOME_UNCERTAIN` | 10 | PASS |
| `INGESTION_ACCEPTED_WHILE_WORKER_OCCUPIED` | `BLOCKER_RUNNING_TARGET_QUEUED` | `TARGET_SUCCEEDED_SEARCHABLE_NO_DUPLICATES` | 10 | PASS |

For the Answer case, the provider had received the complete request while the
durable message was still pending. After forced termination, restart reconciled
exactly one message to `OUTCOME_UNCERTAIN`; the provider attempt count remained
one and the answer/source identities did not change.

For ingestion, one real PDF job was durably running and the target upload was
durably queued before forced termination. Restart recovered exactly the expired
running job; the blocker completed on attempt two and the target on attempt
one. The final Vault had exactly two documents, active revisions, terminal
jobs, distinct blobs and searchable FTS identities, with no duplicate chunk
slot. Every stopped database passed SQLite integrity, foreign-key and FTS5
external-content integrity checks.

## Reproduction

From `v2/` with the pinned Go toolchain:

```powershell
$env:MW_RUNTIME_QUALIFICATION_COUNT = '10'
$env:MW_RUNTIME_QUALIFICATION_REPORT = '<new evidence path>'
go test ./qualification/runtime -run '^TestRuntimeInterruptionQualification$' -count=1 -timeout 10m -v
go test ./qualification/runtime -count=1
go vet ./qualification/runtime/...
```

Observed on the stated commit: 20/20 forced-process subtests passed in 13.87
seconds; the committed evidence contract and normal package run also passed.

## Deliberate limits

- This proves controlled process termination, not power loss, kernel failure,
  controller failure or damaged media.
- It proves two selected persistence windows, not every schema-migration,
  Blob, backup or provider checkpoint. RUN-002 and RUN-003 therefore remain
  `IMPLEMENTED`, not `PASS`.
- Inputs are synthetic and all provider/coordinator traffic is literal
  loopback. No external provider or user Vault was contacted.
- This is not browser, retrieval-quality, performance, soak, installer or
  release qualification.
