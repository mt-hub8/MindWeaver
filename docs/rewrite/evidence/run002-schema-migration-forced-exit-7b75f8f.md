# RUN-002 schema-migration forced-exit boundary

## Scope and ruling

This qualification is based on
`codex/go-rewrite@7b75f8f4735f7ddd54b619dd0dbe73f253fa3a80`.
It adds only a Windows/amd64 qualification test and this evidence. It does not
change an embedded migration, a migration checksum, the production Store or
App, the acceptance ledger, or any Java/MySQL migration surface.

RUN-002 remains `IMPLEMENTED`. The matrix closes the two deterministic process
boundaries available without adding a production or VFS fault hook. It does
not claim to have interrupted SQLite while `COMMIT` itself was in flight.

## Exact matrix

`TestRUN002SchemaMigrationForcedExitMatrix` generates empty historical v1
through v6 Vault databases from the tracked raw bytes of migrations 001 through
007. Each fixture records the raw SHA-256 of every migration already applied.
Production `Store.Open` independently rechecks those values against the
embedded migration set.

Each historical version runs both of these forced-exit cases:

1. `BEFORE_FIRST_PENDING_TRANSACTION`: the parent retains a real
   `BEGIN IMMEDIATE` writer transaction. The child validates the historical
   ledger immediately before calling real `App.Start`; while the child remains
   alive, the parent proves the ledger and ordinary collection set are
   unchanged, then kills and reaps the child. The retained transaction makes a
   pending migration commit impossible at this checkpoint. This is a
   before-transaction boundary, not a claim about the exact blocked instruction
   inside SQLite.
2. `AFTER_ALL_MIGRATIONS_BEFORE_LISTENER`: real `App.Start` completes every
   pending migration, validates the current ledger, performs startup recovery,
   and reaches a qualification-only `ExtraRoutes` callback. The callback proves
   schema v7 and then blocks before `localhttp.Start` creates a listener. The
   parent kills and reaps the child at that exact application boundary.

After every kill, the next real App owner must reach its route-registration
boundary with schema v7 before a listener exists. Only after `App.Start`
returns does the test exchange the real bootstrap/session/CSRF protocol and
perform an ordinary collection write. After shutdown, direct Store reopen and
a second owner must preserve that write while passing:

- exact contiguous v1-through-v7 name and raw-checksum ledger comparison;
- `SchemaVersion == 7`;
- SQLite `integrity_check`;
- `foreign_key_check`;
- the FTS5 external-content integrity command;
- absence of the retired `settings` table; and
- exact one-row ordinary-write identity after reopen.

Child authorization is a single-use 256-bit capability in a parent-created
sandbox. Child stderr is bounded, checkpoint JSON is closed and bounded, and
every child is killed and reaped. No product path, source content, database
value, bootstrap credential, or capability is written to evidence.

## Honest remaining boundary

Each production migration executes its SQL and ledger insert in one
`BEGIN IMMEDIATE` transaction. With no production/VFS seam, an external poll can
observe only the committed old or new ledger; racing a process kill against the
unobservable COMMIT instruction would not be deterministic evidence. Therefore
the following remains unqualified rather than being simulated:

- forced termination after SQLite has begun COMMIT but before the caller has
  observed its outcome; and
- separately stopping after every intermediate commit of one multi-version
  startup. Every intermediate committed schema is covered as the next frozen
  historical fixture, but that is compatibility evidence rather than an
  in-flight forced-exit checkpoint.

Closing those points would require a narrowly reviewed SQLite/VFS fault seam or
an external machine-level fault campaign. This slice deliberately does not add
a generic journal, a migration hook, or checksum-changing SQL.

## Reproduction

From `v2/` with the pinned Go 1.27.0 Windows/amd64 toolchain and offline module
policy:

```powershell
go test ./qualification/runtime -run '^TestRUN002SchemaMigrationForcedExitMatrix$' -count=10
go test ./qualification/runtime ./internal/store/sqlite ./internal/app -count=1
go vet ./qualification/runtime ./internal/store/sqlite ./internal/app
```
