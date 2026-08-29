# REL-001 deterministic HTTP kill/replay campaign

Status: **NOT_IMPLEMENTED**

- Production baseline: `b00363d1258f9c15f9949fc94f592192f7898df7`
- Qualification code: `bf9f99c95687ef7073c50bb61b3485f6f944ffc6`
- Recorded: 2026-08-28 (Asia/Shanghai)
- Production changes: none
- Qualification diff: five files under `v2/qualification/runtime`,
  1,464 insertions and 46 deletions
- Plan SHA-256:
  `70f1d8bf4971192da9db3304fbdf62ea5d9f4d258897a1a6c2664973df6c7906`

This qualification-only campaign is a KEEP developer reliability signal. It
does not promote the conjunctive `REL-001` row and does not change production,
schemas, public contracts, release scripts, vendor contents or frozen product
identity.

## Frozen campaign

The committed plan contains exactly 1,024 frames, with 256 frames for each
real product mutation:

- collection creation;
- conversation creation;
- bounded TXT upload followed by ingestion and search;
- an Ask request that must terminate without calling the configured literal-
  loopback provider.

The campaign builds the current-tree `mindweaver.exe` once. Each frame creates
a fresh Vault, starts that binary, bootstraps a real authenticated HTTP session,
submits one mutation, terminates and reaps the process through a retained
Windows process handle, starts the same binary as a new process with a different
PID and submits the exact request twice more.

The response contract distinguishes an initial mutation from a durable replay:

- collection and conversation creation require `201` plus `created:true`
  initially and `200` plus `created:false` on replay;
- upload requires `202` plus `created:true` initially and `200` plus
  `created:false` on replay;
- Ask has no public `created` field, so it requires `200`, a complete stable
  Answer identity and exact durable cardinality of one `ask_requests` row and
  two conversation messages.

If the first response was completely decoded, the first replay must already be
a replay and must preserve its identity. If the first response was interrupted,
the first retry may be the initial mutation or its replay, but the second retry
must be a replay with the same identity. This prevents deterministic IDs from
masking a lost commit followed by recreation.

After the restarted process is stopped, every frame opens the SQLite file and
checks `integrity_check`, foreign keys, FTS5 external-content integrity and
`CheckCanonicalConsistency`. It verifies exact row counts across all 19 current
logical tables, exact mutation-specific row identities, and the absence of the
removed `settings`, migration-temporary `document_ingestions_pdf`, and legacy
tables. Blob staging must be empty and `blob_gc_candidates` must be zero. Upload
frames require exactly one content-addressed object with the expected prefix,
leaf name, regular-file type, size, bytes and SHA-256; all other mutations
require an empty object tree. Test-sandbox deletion is only harness hygiene and
is not used as the product-residue oracle.

## Decision schedule and process boundary

The plan uses a frozen 256-bit seed and deterministic delays in the inclusive
range 0..100 milliseconds. Sixteen frames whose delay is below two milliseconds
use an explicit first-response-byte barrier, distributed `8/2/2/4` across the
four mutations. `WroteRequest` only reports and returns. Once the server begins
the response, the client blocks before decoding it; the controller records the
terminate-before-complete-response decision, terminates and observes the
process as signaled, waits for `Cmd.Wait`, and only then releases response
decoding.

Every ordinary frame instead waits its frozen delay after a successful request
write. An atomic first-claim records only the order between a completely decoded
response and the controller's decision to terminate. It is not a claimed
SQLite commit-instruction, kernel-sync or power-loss checkpoint. Every actual
termination independently requires `TerminateProcess(..., 1)`, an OS-signaled
retained handle, exact process exit code 1, bounded `Cmd.Wait`, and exactly-once
handle close.

## Executed result

The final opt-in campaign ran from a clean detached worktree at qualification
commit `bf9f99c95687ef7073c50bb61b3485f6f944ffc6`. It used Go 1.27.0 for
Windows/amd64, `CGO_ENABLED=0`, local toolchain selection, vendored module mode,
telemetry disabled, and module/checksum/workspace/VCS network resolution off.
The outer test caches began empty and its module cache remained empty. The
qualification's product build independently used fresh empty module, build and
temporary caches and asserted its module cache remained empty.

| Mutation | Response before decision | Barrier decision before response | Ordinary decision before response | Total |
| --- | ---: | ---: | ---: | ---: |
| collection create | 248 | 8 | 0 | 256 |
| conversation create | 254 | 2 | 0 | 256 |
| TXT upload | 235 | 2 | 19 | 256 |
| Ask without context | 252 | 4 | 0 | 256 |
| **Total** | **989** | **16** | **19** | **1,024** |

All 1,024 subtests ran and passed; there were no failure lines, the aggregate
result appeared exactly once, the campaign test sandbox was empty, and the
before/after process census found no newly surviving `mindweaver.exe` process.
The test took 229.05 seconds and the Go package completed in 231.154 seconds.

The bounded Go-test transcript contains 2,054 lines and 156,403 bytes, with
SHA-256
`dabbb7edaa98f6fbc91e9359717f0c44d95c0a25711323c349bb620a1bf5c863`.
It is stored outside the repository and is neither a signed release artifact
nor committed evidence. The invoking wrapper and a post-run read-only check
separately observed the fresh outer caches, empty outer module cache, 246.094-
second wrapper duration and process census. None of those wrapper observations
is covered by the transcript hash.

## Superseded diagnostic observation

The earlier 1,024-frame run at `915c2e122873bbdeb433ff3cdb72c72a8c730c25`
and transcript SHA-256
`81fc5780115a7bbd81a1c6cc3fb8b41eaba375d04ee5b391e248f040b2c9fe3c`
are explicitly superseded and must not be used as qualification evidence. That
version discarded the first replay's `created` value and lacked raw SQLite/FTS/
Blob final-state checks, so a lost commit followed by deterministic-ID
recreation could have appeared successful. Commit `bf9f99c` closed both gaps
before the final run above.

## Deliberate non-claims

This is a deterministic single-mutation, fresh-Vault process campaign over four
HTTP operations and synthetic content. The 16 barrier frames intentionally
force one observable ordering; among ordinary delayed frames, only upload
observed decision-before-response in this run. The campaign does not claim to
hit an exact SQLite commit instruction, terminate inside filesystem sync, or
certify physical-media durability. Its raw storage oracle proves final
restart/replay convergence, not the location of an unobservable in-flight
commit.

It is not randomized operation-sequence fuzzing, a concurrent scheduler
campaign, a power-loss or physical-media test, a `go test -race` result, a
24–48 hour soak, or an accepted 10k/100k performance and capacity result. It
does not independently bind a signed release binary or machine attestation and
does not produce a durable per-frame evidence artifact. Those required facets
remain open, so `REL-001` remains `NOT_IMPLEMENTED`.

## Reproduction

From `v2/` at the qualification commit, using new empty cache directories:

```powershell
$go = '<pinned-go1.27.0>\bin\go.exe'
$env:MW_REL001_HTTP_KILL_CAMPAIGN = 'RUN_1024_V2'
$env:MW_GO = $go
$env:CGO_ENABLED = '0'
$env:GOTOOLCHAIN = 'local'
$env:GOPROXY = 'off'
$env:GOSUMDB = 'off'
$env:GOWORK = 'off'
$env:GOENV = 'off'
$env:GOVCS = '*:off'
$env:GOTELEMETRY = 'off'
$env:GOFLAGS = '-mod=vendor'
$env:GOMODCACHE = '<new-empty-gomodcache>'
$env:GOCACHE = '<new-empty-gocache>'
$env:GOTMPDIR = '<new-empty-gotmpdir>'
& $go test ./qualification/runtime `
  -run '^TestREL001DeterministicHTTPKillReplay$' `
  -count=1 -timeout 20m -v
```

Without the exact opt-in value, the always-run test executes only eight bounded
smoke frames: one barrier frame and one ordinary frame for each mutation.
