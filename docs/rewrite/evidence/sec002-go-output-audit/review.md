# SEC-002 Go CORE output audit

This is a deletion-oriented review of the reachable Go product at the
integrated output baseline `89be0fbbbbb583d621860531419b3dbc34bf53b3`,
whose parent product baseline is
`4a338dc1ffd22e139703b5f5e4a907bb733e2fbe`. It does not review, run, retain,
or restore any Java, Python, MySQL, legacy exporter, or legacy migration code.
It does not change the acceptance ledger.

`surface.csv` is the exact reviewed set of security-sensitive operational
egress and its classifiers: 28 files, 11,983 lines, and 439,324 bytes. Each row
binds the full file to its SHA-256, byte count, line count, and `1-N` range.
The set was derived from the two production command closures, then reduced to
the files that directly do at least one of the following:

- write CLI, helper, HTTP, provider, or browser output;
- turn internal failures into a public Problem, CLI message, status, or backup
  outcome; or
- persist a failure projection that is later returned by the product.

There is no production telemetry or application log stream. The only standard
library `log` import is `internal/localhttp/server.go`, where `http.Server`'s
`ErrorLog` is fixed to `log.New(io.Discard, "", 0)`. No reachable production Go
file imports `log/slog`. Raw causes remain available through Go error wrapping
for in-process decisions, but the CLI prints only `apperror.PublicMessage`,
business HTTP errors cross only the bounded `safeBusinessResponse` and a
validated Problem, and backup CLI errors use the path-free classified wrapper.

## Finding closed by this slice

Before `89be0fb`, `mindweaver config init` echoed the caller-supplied config
path and `mindweaver config check` echoed the configured Vault root. These
values are unnecessary in a success status and commonly become terminal or
automation logs. Both commands now emit only the fixed operation result and
schema version. `TestConfigInitAndCheckOutputIsPathFree` uses distinct config
and Vault path canaries and requires exact output bytes.

No P0 was found. This was the only open P1 in the reviewed operational egress.

## Existing closed boundaries retained

- `localhttp` uses exact routes, one-use session bootstrap, safe fixed transport
  errors, bounded buffered success responses, and validated Problems. Its
  panic, timeout, oversized body, arbitrary `http.Error`, hostile Host, and
  bootstrap-redaction tests exercise the actual listener.
- The embedded Web UI renders authenticated product data and validated Problem
  details with DOM `textContent`, never HTML interpolation. Its fixed local
  validation errors, stable failure codes, and recovery guidance are the only
  other operational browser output; raw Go/provider/parser errors do not cross
  this boundary.
- App error classification returns fixed details. Resource identifiers are
  body/query values rather than path segments, so `Problem.instance` is one of
  the registered constant route paths. Backup status contains stable IDs,
  phases, counts, and codes, never its destination or raw engine error.
- Ollama is literal-loopback only. Prompt is the intended bounded provider
  request; hostile response bodies, headers, framing, URLs, and model failures
  are collapsed to stable errors and never formatted into a Problem or durable
  error code.
- The reachable RAG, ingestion, and purge services supply fixed controlled
  terminal text and stable codes; SQLite bounds the text and validates the code
  alphabet. They do not persist raw Go/provider/parser errors.
- The PDF helper writes extracted text only to its bounded private stdout pipe.
  Its production failure channel is empty; the client discards the bounded
  hostile stderr buffer and returns a stable category.
- Recovery NDJSON contains only fixed phase names, aggregate counts/bytes, and
  failure classes. It does not contain source, destination, scratch, or Vault
  paths.

## Deliberate data-bearing channels, not diagnostic output

SEC-002 must not be misread as a claim that every product byte is content-free.
The following channels carry user data or credentials by design and are not
logs, diagnostic events, or qualification evidence:

- authenticated API success/history/search responses and the owned Vault;
- plaintext backup payloads, which the CLI and UI identify as plaintext;
- the private `mindweaver-pdf` pipe carrying extracted source text;
- the one-use loopback bootstrap URL printed for the operator and handed to the
  browser, plus the same-origin session/CSRF exchange; and
- the local versioned config file containing the selected Vault root.

Their error and status paths remain content-free. This review does not claim
that a captured terminal, browser profile, backup, or Vault is non-sensitive.

## Reproducible focused checks

Run from `v2/` with the pinned Go 1.27 toolchain and no network:

```powershell
$go = $env:MINDWEAVER_GO
if ([string]::IsNullOrWhiteSpace($go)) { throw 'MINDWEAVER_GO is required' }
& $go test ./cmd/mindweaver -run '^(TestConfigInitAndCheckOutputIsPathFree|TestValidateBrowserLaunchURLAllowsOnlyExactLoopbackBootstrap|TestRecoveryCLIFailureIsPathAndContentFree)$' -count=10
& $go test ./internal/localhttp -run '^(TestStartBindsOnlyRandomIPv4LoopbackAndRedactsBootstrap|TestRawTCPRejectsDNSRebindingHostWithoutLeakingIt|TestOnlyValidatedProblemCanCrossSanitizingBoundary|TestBusinessErrorsAreSanitizedAndBodiesAreBounded|TestRequestTimeoutIsBoundedAndSafe|TestBusinessHandlerPanicIsContainedInsideTimeoutGoroutine)$' -count=10
& $go test ./cmd/mindweaver ./internal/localhttp ./platform/apperror ./internal/ollama ./internal/pdfextract/client ./internal/rag ./internal/app ./internal/webui -count=1
& $go vet ./cmd/mindweaver ./internal/localhttp ./platform/apperror ./internal/ollama ./internal/pdfextract/client ./internal/rag ./internal/app ./internal/webui
```

Those checks were run against the clean candidate `e2d27e9` before this
evidence commit. Its two changed code blobs are byte-identical to the
integrated `89be0fb` blobs. All four focused canary groups passed at
`-count=10`; the eight complete packages and their focused `go vet` set also
passed. The CSV was independently recomputed against the integrated baseline:
28 unique paths, with every SHA-256, byte count, line count, and range matching.
The omitted browser boundary was found by an independent deletion review;
`internal/webui` tests and vet then passed against the same unchanged product
blobs before its two rows were added here.

## Still required at final release scope

This source and runtime boundary does not qualify release-wide artifacts.
SEC-002 must remain short of final release closure until the final exact dual-PE
build and installer environment perform a bounded canary scan over stdout,
stderr, installer and OS event logs, crash/error artifacts, signed manifests,
SBOM/NOTICE, browser screenshots/traces, and published qualification reports. Browser
screenshots/traces are currently not produced because real browser
qualification is blocked; no fake harness result may replace that evidence.
The release scan must distinguish the deliberate data-bearing channels above
instead of either leaking them or falsely describing them as content-free.
