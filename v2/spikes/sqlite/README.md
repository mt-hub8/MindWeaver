# SQLite Windows feasibility spike

This directory is an isolated, deletion-safe experiment for
`github.com/ncruces/go-sqlite3 v0.35.3`. It produces reproducible evidence; it
does **not** select a production driver or change the v2 product module.

The spike has its own `go.mod`. Removing `v2/spikes/sqlite` removes the entire
experiment. Runtime databases, WAL/SHM files, process markers, and binaries are
created only under the OS temporary directory. The two subprocess helper
commands reject database paths outside that directory.

## Run on Windows

From the repository root:

```powershell
pwsh -File .\v2\spikes\sqlite\run.ps1
```

The runner finds `go.exe` on `PATH`, then checks the Codex desktop Go 1.27
toolchain below `%LOCALAPPDATA%`. It performs these steps with `CGO_ENABLED=0`:

1. requires a clean `gofmt -l` result;
2. runs `go mod verify`;
3. runs `go test -count=3 ./...`;
4. builds a temporary probe executable with `-trimpath`;
5. runs every probe, including two real subprocesses and forced holder death;
6. writes machine-readable JSON under the repository-relative ignored path
   `v2/spikes/sqlite/evidence/local-<timestamp>.json` by default.

To write an evidence file at a repository-relative location:

```powershell
pwsh -File .\v2\spikes\sqlite\run.ps1 `
  -OutputPath v2\spikes\sqlite\evidence\local.json
```

`-Race` is opt-in because the Go race runtime on Windows requires a supported C
toolchain. The selected candidate itself is CGO-free; the ordinary runner
proves that by building and testing with `CGO_ENABLED=0`.

On the checked-in evidence host, `go test -race -count=1 ./...` was attempted
with `CGO_ENABLED=1` and stopped in `runtime/cgo` because `gcc` was not present
on `PATH`. Race coverage is therefore **not verified** by this capture. That is
an unmet environment gate, not a candidate pass (and not a candidate runtime
failure). No CGO-backed comparison candidate was built.

Manual equivalent (inside this directory) is:

```powershell
$env:CGO_ENABLED = "0"
go mod verify
go test -count=3 ./...
go build -trimpath -o "$env:TEMP\sqlite-spike.exe" .\cmd\sqlite-spike
& "$env:TEMP\sqlite-spike.exe" run --json "$env:TEMP\sqlite-spike.json"
```

## Evidence collected

The JSON has a schema version, candidate/build identity, Go runtime identity,
SQLite version and full `sqlite_source_id()`, total and per-probe elapsed time,
and Windows process RSS from `GetProcessMemoryInfo` (start/current/peak working
set). It contains no machine-user paths. Runtime sidecars are reported only by
probe-relative basename.

The executable exercises:

- connection initialization and readback on three simultaneous physical
  connections: `foreign_keys=1`, `journal_mode=wal`, `synchronous=FULL` (numeric
  `2`), the requested `busy_timeout`, and `trusted_schema=0`; it also proves the
  foreign-key setting with an actual rejected insert;
- a dynamically registered FTS5 extension with a real virtual table,
  insertion, `MATCH`, `highlight`, and `bm25` query;
- one pinned writer and three pinned readers in WAL mode, including a writer
  commit while a read snapshot stays open;
- interruption of a long recursive query and a successful health query on the
  same physical connection immediately afterward;
- `SQLITE_FULL` induced by `max_page_count`, followed by verification that the
  surrounding transaction leaked no rows;
- incremental online `BackupInit`/`Step`, `Restore`, row/checksum validation,
  and `PRAGMA integrity_check`;
- stable classification based on raw numeric primary and extended SQLite result
  codes (for example BUSY `5`, INTERRUPT `9`, FULL `13`, CONSTRAINT `19`, and
  observed UNIQUE `2067`/FOREIGN KEY `787` extended codes);
- two separate processes contending for a writer lock, with requested timeout,
  accepted elapsed-time bounds, actual elapsed time, raw BUSY codes, forced
  holder termination, relative WAL/SHM sidecar state, lock reacquisition,
  rolled-back uncommitted data, and reopen `integrity_check`.

## Interpretation limits

Passing means this pinned build demonstrated the listed mechanics on the test
machine. It is not a durability proof against power loss, a production workload
benchmark, or a driver-selection decision. Process termination and power loss
have different failure modes. RSS includes the Go process and loaded Wasm/FTS5
runtime, not just SQLite pages.

The optional `modernc.org/sqlite v1.57.0` / `modernc.org/libc v1.74.4`
build-tag comparison is deliberately not included, so its dependency download
cannot block the primary evidence. A fair comparison should be a separate run
with identical workload, cache state, and machine controls.

The checked-in JSON under `evidence/` is one reproducible capture, not a golden
test: timestamps, timings, RSS, SQLite source ID, and WAL/SHM cleanup details may
legitimately differ after a pinned dependency rebuild or on another machine.
