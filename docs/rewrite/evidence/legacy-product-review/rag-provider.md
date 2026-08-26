# RAG and provider source review

## Frozen baseline and scope

- Baseline: `0df22ddaf02c64bf73a7df12cd5fea6b52632c73`
- Branch: `codex/legacy-product-review-0df22dd`
- Machine-readable evidence: [`rag-provider.csv`](./rag-provider.csv)
- Verification: `pwsh -NoProfile -File docs/rewrite/evidence/legacy-product-review/verify.ps1`
- Coverage convention: every manifest row covers the complete file, from line
  `1` through the recorded `line_end`. SHA-256 is over the checked-out bytes at
  the frozen baseline. No declaration or hand-selected line is omitted inside a
  listed file.

The legacy set is the union of all production files under `rag`, `grounding`,
`llm`, and `modelprovider`; all tests under the corresponding package paths;
and the named controller, service, DTO, configuration, entity, repository,
secret, schema, and static-browser adapters that expose those packages. Tests
whose subject is an adapter outside the direct package are included explicitly.
Agent, Memory, Evaluation, Batch, and KB Health consumers are not silently
absorbed into this set; they are separate review partitions. Import edges from
RAG into Memory, advanced retrieval, embedding, and vector code are evidence of
coupling, not authorization to retain those implementations.

## Coverage result

| Surface | Files | Lines | Decision |
| --- | ---: | ---: | --- |
| Legacy direct production packages | 58 | 3,209 | `DROP` |
| Legacy product adapters | 26 | 2,642 | `DROP` |
| Legacy MySQL schema | 1 | 25 | `DROP` |
| Legacy static UI | 8 | 2,213 | `DROP` (workflow vocabulary is evidence only) |
| Legacy tests | 29 | 2,865 | `DROP` |
| Current Go production/schema/tests | 7 | 3,600 | `KEEP` |
| Current Go production/UI/tests/contracts | 12 | 5,603 | `HARDEN` |
| **Total** | **141** | **20,157** | frozen in CSV |

There are zero Java implementation exceptions. The tests demonstrate units,
Spring wiring, mock HTTP/controller behavior, and documentation strings. They
do not exercise an authenticated browser, a hostile network, a durable answer
restart, or a provider-call crash boundary as one closed loop.

## Legacy findings

### P0: arbitrary provider authority can receive a stored credential

`ModelProviderController.java:54-89` exposes create/update/test/default
mutations without an application authentication or CSRF boundary in this
surface. `ModelProviderConfigService.java:218-246` checks that a URL and key are
present but does not restrict scheme, authority, address class, redirects, or
ambient proxy behavior. `ModelProviderTestService.java:98-110` then decrypts the
stored key and sends it as `Authorization: Bearer` to that configured base URL.
The generation client repeats the same authority trust at
`RestClientOpenAiCompatibleLlmHttpClient.java:23-37`. This is an SSRF and
credential-exfiltration path, not a reusable provider abstraction.

### P1: post-verification refusal is computed but not enforced

`RagAnswerService.java:353-383` computes citation verification, unsupported
claims, and a post-verification refusal. The response at lines `385-397`
nevertheless always returns `llmResponse.getContent()` and never substitutes
the refusal or changes the outcome. A failed verification can therefore remain
the user-visible answer.

### P1: heuristic `UNKNOWN`/`WEAK` can be reported as valid

`CitationVerificationService.java:71-101` counts only `UNSUPPORTED` as failed.
`UNKNOWN` and `WEAK` are not verified, but neither increments `unsupported`, so
the aggregate `valid` predicate can still be true. The result is not durably
bound to an answer/context identity, and keyword overlap does not establish
semantic support. The implementation and its truth-sounding diagnostics are
discarded; only the requirement that citations refer to final supplied chunks
survives in the Go product.

### P1: provider completion and retry semantics are not durable

`RagAnswerController.java:27-29` invokes one synchronous service call. The
legacy path has no conversation/message reservation, idempotency identity,
pending terminal state, or restart reconciliation. The OpenAI-compatible client
at `RestClientOpenAiCompatibleLlmHttpClient.java:25-29` configures only connect
timeout, while the local Python client has no explicit transport timeout. A
timeout or process exit cannot distinguish a definite failure from an uncertain
remote outcome.

## Current Go boundary

### KEEP

- `internal/ollama`: a concrete credential-free, literal-loopback client. Its
  tests exercise real HTTP sockets, fixed-address dialing, ignored ambient
  proxy, redirect rejection, response bounds, cancellation, timeout, malformed
  framing, and connection reuse.
- `internal/store/sqlite/rag_answers*` and migration `004`: durable
  conversations/messages/sources, provider-config version binding,
  idempotency, revision checks, source revalidation, pending reconciliation,
  and restart pagination are exercised against SQLite.
- `internal/app/api_cursor*`: bounded stable pagination is shared by the
  conversation surface and has executable tests.

### HARDEN

- `internal/rag` and its app/API runtime remain the accepted vertical Ask path,
  but their validator proves citation syntax and membership only. It does not
  prove that every substantive statement is supported.
- The Web UI says “验证引用” at `internal/webui/static/app.js:724`, `:750`, and
  `:943`, while `internal/rag/service.go:416-455` only parses in-range numeric
  references. The copy must say structural/in-scope citation validation, or a
  separately accepted support verifier must exist. This is a P1 product-trust
  hardening item, not grounds to revive Java grounding heuristics.
- The embedded UI tests inspect shipped assets and client logic but do not run
  a real browser. Browser session, keyboard/focus, Chinese input, scale/high
  contrast, offline/no-model, and recovery workflows remain qualification work.
- The OpenAPI contract stays incremental and must track only running CORE
  handlers. No Agent, vector, Evaluation, Batch, or migration route is admitted.

No current Go file in this partition is marked `REWRITE`. `HARDEN` means retain
the working vertical slice and close the named user-observable gap; it does not
permit adding a generic provider registry, semantic-judge model, invocation
proof hierarchy, Agent, or vector subsystem.

## Executed checks

```text
go1.27.0 test ./internal/rag ./internal/ollama ./internal/app ./internal/store/sqlite ./internal/webui ./openapi/v1
PASS
```

The evidence verifier and `git diff --check` are commit gates. No test called a
real model or used user data.
