# MindWeaver CORE implementation contract

This directory contains the versioned, implementation-bound HTTP and
production-surface contracts for the local Go workbench.

`openapi.json` is the canonical OpenAPI 3.1 document. It describes exactly the
27 fixed `/api/v1` operations registered by `internal/app`, plus the four
fixed transport operations implemented by `internal/localhttp` (bootstrap
exchange, CSRF refresh, and GET/HEAD health). Static UI assets are intentionally
outside OpenAPI. There are no path parameters, wildcard routes, SSE endpoints,
accounts, remote authentication, browser restore endpoints, or LATER
Agent/vector protocols.

The local transport has two distinct error boundaries:

- An authenticated business handler returns the versioned RFC 9457-shaped
  `Problem` body with a stable code/status mapping and
  `application/problem+json`.
- Host, Origin, loopback-peer, Fetch Metadata, session, CSRF, body-limit, and
  method rejection happens before a business handler and returns only the
  content-free `{"error":"request rejected"}` transport body. The OpenAPI
  `Rejected*` responses preserve that distinction.

The browser session cookie name contains process-random entropy and therefore
cannot truthfully be represented as a fixed OpenAPI cookie security-scheme
name. The top-level `x-mindweaver-auth` contract instead freezes the actual
bootstrap header, random cookie shape, process-memory lifetime, CSRF header,
refresh route, and same-origin enforcement. Every business write requires the
CSRF header. Four durable create operations additionally require
`Idempotency-Key`.

The production API does not implement ETag or `If-Match`. Optimistic
concurrency is carried by `expectedRevision` or `expectedVersion` in strict
JSON bodies. The contract explicitly marks ETag unsupported, and the validator
rejects an invented ETag/`If-Match` parameter or response header.

`core-surface.v1.json` is the fail-closed CORE allowlist. It covers production
Go library packages, HTTP operations, migration filenames, application-declared
final SQLite tables, and executable command surfaces. Package discovery scans
only non-test `.go` files under `internal/` and `platform/`; `_test.go` files
and root-level `docs`, `migration`, `openapi`, `qualification`, `release`,
`spikes`, and `testdata` evidence/tooling roots are not production packages.
The SQLite list covers application-declared tables, including the FTS virtual
table, but not SQLite-owned `sqlite_*` or FTS shadow implementation tables.
Exact token matching prevents Agent, memory, vector, embedding, rerank,
evaluation, notification, and reindex surfaces without misclassifying accepted
document revisions, ingestion jobs, Answer sources/citations, or Ollama calls.

`contract.go` uses only the standard library. It validates bounded UTF-8,
duplicate-free canonical JSON; rejects unknown fields and unsupported schema
vocabulary; resolves every local reference; enforces unique bounded
operationIds and exact methods/paths; and cross-binds session, CSRF,
idempotency, success, Problem, and transport status metadata to the surface
manifest. `contract_test.go` then extracts the real app route literal with the
Go AST, compares the local transport allowlist, discovers production packages,
derives the final declared table set across ordered migrations, and inspects
the real CLI switches.

This is implementation-closure evidence for API-001 and ARC-003. It is not a
browser compatibility, accessibility, usability, penetration-test, release,
or signed-artifact result, and by itself must not mark UI or SEC acceptance
gates as passed.
