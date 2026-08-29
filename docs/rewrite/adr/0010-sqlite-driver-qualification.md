# ADR 0010: SQLite driver qualification

- Status: Accepted for candidate order; production driver remains Gate 1 pending
- Date: 2026-08-23

## Context

MindWeaver requires a single-process, local-first SQLite store with WAL,
context cancellation, online backup, FTS5, deterministic error classification,
and an offline Windows build. A feature checklist is insufficient: driver VFS,
shared-memory, compiler, memory, and crash behavior are part of data safety.

## Decision

Production code depends only on a narrow storage adapter for open, connection
initialization, read/write transactions, backup, checkpoint, health, and numeric
error normalization. Driver types, DSN syntax, and raw connections cannot enter
domain packages.

The qualification order is:

1. `github.com/ncruces/go-sqlite3 v0.35.3` as the primary pure-Go candidate.
2. `modernc.org/sqlite v1.57.0` with exactly matching
   `modernc.org/libc v1.74.4` as the pure-Go comparison/fallback.
3. `github.com/mattn/go-sqlite3 v1.14.50` as the CGO differential oracle and
   fallback if neither pure-Go candidate meets the reliability gate.

This order is not production acceptance. `ncruces` is pre-v1 and its Wasm
sandbox increases per-connection memory; it must pass RSS, cold build, and long
Windows WAL tests. `modernc` is stable and capable but remains blocked as the
default candidate by the unresolved Windows WAL/SEH risk in upstream issue
221. `mattn` adds a Windows compiler and packaging burden but supplies a mature
semantic and performance comparison.

The initial runtime uses one serialized writer connection with short
`BEGIN IMMEDIATE` transactions and a small measured reader pool. It does not
enable SQLite shared-cache. Every physical connection applies and reads back:

```sql
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;
PRAGMA synchronous = FULL;
PRAGMA busy_timeout = 5000;
PRAGMA trusted_schema = OFF;
```

Read-only connections also use `query_only=ON`. Startup records and verifies
SQLite version/source ID, compile options, all required pragmas, and a real FTS5
create/drop probe. Unknown or silently ignored configuration fails startup.

The active database obeys ADR 0009's local fixed-volume policy. WAL is never
used over a network filesystem.

## Error and recovery semantics

Adapters classify numeric primary and extended SQLite result codes, never error
text:

- `BUSY`/`LOCKED`: retry the complete idempotent transaction within a bound;
  `BUSY_SNAPSHOT` starts a new transaction.
- `INTERRUPT`: cancellation; the connection must remain usable.
- `FULL`: `RESOURCE_LIMIT`, with no partial business transition.
- `CORRUPT`/`NOTADB`: isolate and enter recovery; never auto-overwrite.
- commit-adjacent `IOERR`: `OUTCOME_UNCERTAIN`; reconcile by operation identity
  before any replay.

Online database backup is only one part of a Vault backup. A backup epoch pins
blob deletion, the SQLite Backup API creates a snapshot, the manifest is read
from that snapshot, every referenced immutable blob is copied and hashed, and
the result passes database and application integrity checks before atomic
publication. Restore builds a new Vault and never merges over an open Vault.

## FTS decision boundary

The driver choice does not freeze a custom SQLite tokenizer into the database
format. Gate 1 compares:

1. raw `unicode61`;
2. built-in `trigram`;
3. deterministic application-side Chinese/mixed-language tokenization stored in
   a derived FTS column while citations continue to use original text offsets.

Tokenizer name, version, configuration, and dictionary digest are generation
fingerprints. A change rebuilds derived indexes. A custom FTS5 tokenizer remains
deferred unless the measured corpus proves the three portable paths inadequate.

## Qualification gate

All candidates run the same adapter conformance suite. Required evidence
includes:

- 10k and 100k representative chunks; cold/hot latency, RSS, executable size,
  and build dependencies;
- one writer with 1/4/8 readers, bounded BUSY handling, and no partial writes;
- cancellation followed by successful connection reuse;
- deterministic `SQLITE_FULL`, corruption/truncation, and backup interruption;
- online backup under writes, 100 restore/integrity rehearsals, and blob-manifest
  verification;
- at least 1,000 random process kills around acknowledged and unacknowledged
  operation IDs;
- 24–48 hours of WAL, checkpoint, backup, Defender/EDR, and random-kill stress on
  supported Windows 10 and 11 machines;
- race, operation-sequence fuzz, migration fixtures, and numeric error mapping.

No integrity error or acknowledged transaction loss is acceptable. Any
committed-but-unacknowledged transaction must be discoverable by operation ID.
Thresholds for latency and memory are versioned with the benchmark machine and
product SLO; they cannot be replaced by a developer-laptop anecdote.

## Authoritative references

- [`ncruces/go-sqlite3` package](https://pkg.go.dev/github.com/ncruces/go-sqlite3)
- [`ncruces` Windows support matrix](https://github.com/ncruces/go-sqlite3/wiki/Support-matrix)
- [`ncruces` v0.35.3 Windows WAL fix](https://github.com/ncruces/go-sqlite3/releases/tag/v0.35.3)
- [`modernc.org/sqlite` package](https://pkg.go.dev/modernc.org/sqlite)
- [`modernc` Windows WAL issue 221](https://gitlab.com/cznic/sqlite/-/work_items/221)
- [`mattn/go-sqlite3` build features](https://github.com/mattn/go-sqlite3#features)
- [SQLite WAL](https://www.sqlite.org/wal.html)
- [SQLite result codes](https://www.sqlite.org/rescode.html)
- [SQLite Online Backup API](https://www.sqlite.org/c3ref/backup_finish.html)
- [SQLite FTS5](https://www.sqlite.org/fts5.html)
