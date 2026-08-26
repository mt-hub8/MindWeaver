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
final SQLite tables, and executable command surfaces. Package discovery runs
the frozen Go tool offline for Windows/amd64 with CGO disabled and derives the
actual transitive closure of the two shipped commands. Every module-local
dependency must be in the exact `internal/` or `platform/` allowlist; excluded
evidence/tooling roots cannot enter either command. The same closure freezes
the exact external module version and h1 set, so a MySQL/JDBC/Flyway module or
any other undeclared dependency fails the contract. Target-aware inspection of
the primary and nested Go modules is paired with an all-source package-clause
scan (including Go-ignored dot, underscore, and `testdata` directories); both
permit only the two shipped main packages and the two named
qualification/spike helpers. `cmd/` itself cannot contain files or a third
command.

External modules with `mysql`, `mariadb`, `jdbc`, or `flyway` path segments are
unconditionally forbidden. They cannot be admitted by merely updating an
allowlist; this preserves the fresh-Vault decision while leaving normal Go
SQLite schema migrations intact.

Each shipped command also binds a canonical `mindweaver.command-source.v1`
manifest. It covers every transitive first-party source and embedded file
selected by that command's Windows build (including `browser_windows.go`,
SQLite migrations, and WebUI assets), its owning import path,
module-relative path, byte size, and SHA-256. Each command separately binds its
first-party package and external module closure before their exact union is
compared with the global surface. Textual source normalizes uniform CRLF to LF solely for
cross-platform checkout stability; mixed line endings fail closed, while
embedded and binary inputs remain byte exact. Adding, removing, moving, or
changing compiled command code changes the checked-in manifest digest.
The contract test separately requires the legacy migration root, importer
package, migration command, SQLite adapter, `007_legacy_import.sql`, and stable
legacy-import production tokens to be absent using ASCII case-insensitive
matching. `internal/backup` is reachable only from the public `mindweaver`
startup recovery command; the PDF helper must not link it. The exact closure
and source manifest therefore change whenever recovery wiring changes. Final
PE and installer contents still require the release artifact gate.
The SQLite list is read from a real freshly migrated database and covers every
application-declared table, including the migration ledger and FTS virtual
table, but not the exact SQLite-owned internal set or the four FTS5-managed
storage tables (`config`, `data`, `docsize`, and `idx`). The bundled runtime
reports those four as `table` and `chunks_fts` as `virtual`; the gate freezes
that exact shape and runs the FTS external-content integrity check. A missing
storage table, type drift, or an extra `chunks_fts_content` table fails closed.
Exact token matching prevents Agent, memory, vector, embedding, rerank,
evaluation, notification, and reindex surfaces without misclassifying accepted
document revisions, ingestion jobs, Answer sources/citations, or Ollama calls.

`contract.go` uses only the standard library. It validates bounded UTF-8,
duplicate-free canonical JSON; rejects unknown fields and unsupported schema
vocabulary; resolves every local reference; enforces unique bounded
operationIds and exact methods/paths; and cross-binds session, CSRF,
idempotency, success, Problem, and transport status metadata to the surface
manifest. `contract_test.go` then extracts the real app route literal with the
Go AST, compares the local transport allowlist, derives the Windows command and
module closure, verifies command source manifests, derives the final declared
table set across ordered migrations, and inspects the real CLI switches and
fresh-Vault absence boundary.

This is implementation-closure evidence for API-001 and ARC-003. It is not a
browser compatibility, accessibility, usability, penetration-test, release,
or signed-artifact result, and by itself must not mark UI or SEC acceptance
gates as passed.
