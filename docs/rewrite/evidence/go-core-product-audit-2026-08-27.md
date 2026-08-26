# Reachable Go CORE product audit

## Identity and exact scope

This read-only audit is frozen at
`codex/rag-boundary-d61@8a62f8f018904337ba0de4fd0697c3b36bf43e5d`.
Its committed product baseline is
`codex/go-rewrite@d61a158afdc7c86914c70a847ce4e9d6fe686517`;
the commits above that baseline change tests and evidence only. The production
files, reviewed line spans, byte hashes, and decisions are recorded in
`go-core-product-exact-set-2026-08-27.csv`.

The audited surface is the Windows `mindweaver` entry point and the reachable
local HTTP, RAG, Ollama, conversation, WebUI, and OpenAPI paths needed for the
current CORE product. The shared App and WebUI files were inspected only for
those paths. Backup-specific handlers (`api.go:167-232` and
`api.go:1037-1053`), backup behavior embedded in shared startup/UI files, PDF
extraction, release, migration, and vendor are outside this decision. Keyword
search is observed as the existing RAG input and is not reinterpreted or
changed.

The browser qualification chain remains test/evidence-only and fail-closed.
Its empty approval cannot start a browser, driver, or product process and is
not evidence for UI-001 or UI-002.

## Closed boundaries

### No context and citation structure

- An empty scoped or unscoped keyword result terminates as `refused` with
  `NO_CONTEXT` before provider I/O. The product HTTP test observes no increase
  in the fake provider call count.
- The prompt treats retrieved text as untrusted evidence. Published citations
  must use the exact bracket grammar, refer only to the bounded ordered source
  set, and pass the durable source identity/content-hash checks at commit.
- The browser copy claims only that citation numbers point to the material used
  for the request. It does not claim semantic, factual, or entailment
  verification. Complex claim support therefore remains explicitly outside
  the present structural citation boundary.

### Session, CSRF, and local transport

- The server binds an OS-selected port on literal `127.0.0.1`, then verifies
  the exact Host, Origin, remote address, Fetch Metadata shape, path, and
  request framing. Browser credentials never authorize a non-loopback peer.
- Bootstrap is random, time-bounded, accepted exactly once under a lock, and
  replaced by one process-memory session. The cookie is random-name,
  `HttpOnly`, `SameSite=Strict`, and path-scoped; shutdown invalidates the
  server-side session.
- Mutations require the matching CSRF header. CSRF refresh requires an
  authenticated same-origin browser fetch and cannot be obtained through a
  top-level navigation or the local read-only client shape.
- Handler panics, timeouts, provider failures, and malformed inputs produce
  bounded fixed errors. Tests reject prompt/body/credential canaries in error
  output. Successful document, source, and answer bodies intentionally remain
  user-visible product data and are not error telemetry.

### Idempotency, cancellation, restart, and uncertain outcomes

- The backend binds durable mutation reservations to canonical request bytes;
  conflicting reuse is rejected and exact replay returns the prior resource.
  Ask admission rejects an over-16-KiB body before reservation or provider I/O.
- The Ollama client uses a fixed literal-loopback dial target, disables ambient
  proxying, refuses redirects, bounds headers and bodies, parses hostile JSON
  fail-closed, and propagates deadline/cancellation without returning provider
  text in errors.
- Once a durable Ask crosses the provider boundary, cancellation, timeout, or
  a post-response commit failure converges to `OUTCOME_UNCERTAIN`. Startup
  reconciliation happens before listener publication. Close/reopen plus exact
  replay returns the same answer identifier and does not call the provider
  again.

## Remaining findings

No production P0 was found. The remaining production/integration P1 set is:

1. **OpenAPI production identity is stale.** The committed source manifest is
   `325dfec05db7f7744c6aaf4de792d80efaa6b458a0b850a635d3c41e3596e099`,
   while the current `mindweaver` production closure calculates
   `7de68fba33368f4cfa74a44cb27a0c5add95c688f70c0c77b939bbb21d825dca`.
   `openapi/v1.TestEmbeddedContractMatchesProduction` therefore blocks the
   full gate. This branch does not rewrite the concurrently owned contract.
2. **OpenAPI response evolution and state invariants are under-specified.**
   Response objects and `Problem` are closed with `additionalProperties:false`;
   the validator requires the closed Problem shape, which makes an additive
   response field a v1 breaking change. `Problem` also has no explicit stable
   retry/user-action fields. `Answer` and `Message` expose one flat status enum
   without discriminated pending/completed/refused/failed shapes, so the
   contract permits combinations that the runtime correctly rejects. Requests
   should remain closed; response/Problem compatibility and terminal state
   shapes need the owning OpenAPI rewrite.
3. **WebUI mutation attempts are not consistently bound to immutable visible
   payloads.** Upload, collection creation, and conversation creation retain
   only an idempotency key; their input listeners can clear it while a request
   is in flight. A lost response followed by an edit can therefore create a
   second live attempt instead of making the prior outcome explicit. Ask does
   retain `{key, body}`, but leaves question/scope editable while uncertain and
   retries the frozen old body even when the form displays new values. The
   backend remains idempotent; the browser workflow needs one immutable
   attempt object per mutation plus locked or explicitly abandoned inputs.

External approved browser/driver artifacts, project licensing, MSI packaging,
and signing certificates are excluded from this P0/P1 count as directed.

## Offline evidence

All Go commands used the pinned Go 1.27.0 Windows amd64 toolchain with
`GOTOOLCHAIN=local`, `GOPROXY=off`, `GOSUMDB=off`, readonly modules, no model,
no user Vault, and no external network.

| Gate | Result |
| --- | --- |
| RAG scope/citation/uncertainty/reopen focused tests, `count=10` | PASS |
| App product/restart/shutdown/strict-JSON focused tests, `count=10` | PASS |
| local HTTP bootstrap/session/CSRF/redaction focused tests, `count=10` | PASS |
| Ollama origin/proxy/redirect/bounds/hostile-JSON/redaction focused tests, `count=10` | PASS |
| `go test` and `go vet` for localhttp/transport/ollama/rag/app/webui | PASS |
| `go vet ./...` | PASS |
| `go test ./...` | BASELINE BLOCKED only by the exact OpenAPI manifest mismatch above; every other package PASS |
| `scripts/ci.ps1` | BASELINE BLOCKED at the same test; every preceding package PASS and temporary build tree removed |
| `scripts/verify-standalone.ps1` | BASELINE BLOCKED at the same test; standalone tree removed |

The fake loopback provider and temporary Vaults qualify deterministic protocol
and durability boundaries only. They do not qualify a real Ollama build, a
real browser, existing user data, forced OS process-tree termination, or
semantic citation correctness.
