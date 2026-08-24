# SQLite DB-002 qualification evidence

- Recorded: 2026-08-24 (Asia/Shanghai)
- Acceptance row: `DB-002`
- Evidence scope: deterministic developer qualification of the selected
  `ncruces/go-sqlite3` storage path
- Ledger effect: none; this report does not change
  `docs/rewrite/acceptance-ledger.md`
- Base repository revision at the time of the run:
  `d4e9a1d09a45181514c214b0ad95856a27ac99d6`

This report records real SQLite file, WAL, locking, quota, corruption, FTS, and
process-exit behavior. The executable evidence is
`v2/internal/store/sqlite/qualification_fault_test.go`. It changes no
production storage API.

## Environment

| Item | Recorded value |
| --- | --- |
| Operating system | Microsoft Windows 11 家庭版 中文版, 10.0.26200 build 26200, x64 |
| CPU | Intel(R) Core(TM) Ultra X7 358H |
| PowerShell | 7.6.4 |
| Go | go1.27.0 windows/amd64 |
| `CGO_ENABLED` | `0` |
| SQLite Go driver | `github.com/ncruces/go-sqlite3 v0.35.3` |
| SQLite runtime | 3.53.4 |
| SQLite source ID | `2026-07-24 19:02:57 bf7c7f30031888f4e796e429ab3978879485813aaca6f641c7b33e4e09459bcc` |
| Filesystem location | `D:` local fixed disk (`DriveType=3`), NTFS |

Environment commands:

```powershell
Set-Location D:\Work\MindWeaver\v2
$mwGo = 'C:\Users\24281\AppData\Local\MindWeaver\toolchains\go1.27.0\bin\go.exe'
& $mwGo version
& $mwGo env GOOS GOARCH CGO_ENABLED GOMOD
& $mwGo list -m github.com/ncruces/go-sqlite3
$mwOS = Get-CimInstance Win32_OperatingSystem
$mwCPU = Get-CimInstance Win32_Processor | Select-Object -First 1
$mwDisk = Get-CimInstance Win32_LogicalDisk -Filter "DeviceID='D:'"
$mwOS | Select-Object Caption, Version, BuildNumber
$mwCPU | Select-Object Name
$mwDisk | Select-Object DeviceID, DriveType, FileSystem, VolumeName, Size, FreeSpace
[System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
$PSVersionTable.PSVersion
git rev-parse HEAD
```

## Exact test commands

Focused fault run:

```powershell
Set-Location D:\Work\MindWeaver\v2
$mwGo = 'C:\Users\24281\AppData\Local\MindWeaver\toolchains\go1.27.0\bin\go.exe'
& $mwGo test -run '^TestQualification' -count=1 -v ./internal/store/sqlite
```

Repeat and static-analysis commands used after the focused run:

```powershell
& $mwGo test -run '^TestQualification' -count=5 ./internal/store/sqlite
& $mwGo test -count=1 ./internal/store/sqlite
& $mwGo vet ./internal/store/sqlite
```

The exact page counts, WAL byte count, and elapsed wait are observations, not
portable performance thresholds. The automated tests assert safety invariants
and the configured busy-wait behavior; they do not define a workstation latency
SLO.

## Recorded observations

### WAL readers, one writer, and busy timeout

Three distinct physical connections were retained from one four-connection
pool. A read transaction established a one-row snapshot. The first serialized
writer inserted a second row without blocking that reader. While the first
writer remained open, a second serialized writer failed with SQLite's numeric
`BUSY`/`LOCKED` classification after the configured wait. After the first
writer committed, the original reader still saw its one-row snapshot; a new
reader saw two rows. The second writer then retried and committed the third row.

Recorded values from the focused run:

```text
journal_mode=wal
busy_timeout_ms=250
observed_busy_wait=250.1169ms
```

This demonstrates WAL reader/writer coexistence and writer serialization. The
250ms setting is intentionally short so the fault test remains bounded; it is
not the production default or a performance acceptance threshold.

### Deterministic `SQLITE_FULL` and transaction atomicity

The test checkpointed the WAL, read the real database page geometry, and set
`PRAGMA max_page_count` to the current page count. It then started the Store's
real immediate transaction, changed a business-state field, and attempted a
`zeroblob` allocation larger than all available free pages. SQLite returned the
numeric primary result `SQLITE_FULL`. The transaction helper rolled back: the
state remained `before`, and the oversized row was absent.

Recorded values:

```text
page_size=4096
page_count=72
freelist_count=3
max_page_count=72
rejected_bytes=77824
result_code=SQLITE_FULL
```

This is SQLite's reliable logical capacity fault. It proves handling of
`SQLITE_FULL` and no half-committed transaction without consuming the host's
physical disk.

### Database corruption and FTS divergence

For database corruption, the test cleanly closed a real database, overwrote and
synced its 16-byte SQLite header, and attempted the normal Store open path. The
driver returned numeric `NOTADB`; the Store did not replace or rewrite the
corrupt file.

For derived-index divergence, the test ingested a real document/chunk/FTS row,
verified a healthy application integrity check, and deliberately removed the
FTS index contents. `PRAGMA integrity_check` still returned `ok`, while the
Store's explicit FTS5 external-content integrity command rejected the
divergence. This confirms why structural integrity alone is insufficient for
the rebuildable FTS derivative.

### Forced process exit and reopen

The parent test created and closed a database containing one committed row. It
then launched the same Go test executable as a child. The child opened the real
Store, began an immediate transaction, inserted enough 16KiB rows with a small
page cache to spill uncommitted frames into WAL, and called `os.Exit(86)` without
Commit, Rollback, Store.Close, or deferred cleanup.

Recorded values:

```text
child_exit_code=86
uncommitted_wal_bytes=4334272
reopened_rows=1
integrity=ok
```

Normal Store reopen recovered the database, retained the one acknowledged row,
discarded all uncommitted child rows, and passed structural, foreign-key, and
FTS integrity checks.

## Claim boundary and remaining qualification

This evidence deliberately does **not** claim any of the following:

- physical-disk exhaustion, filesystem quota exhaustion, controller cache
  behavior, torn sectors, or sudden power loss;
- commit-adjacent `IOERR` outcome reconciliation;
- 1,000 randomized process kills;
- 24–48 hour WAL/checkpoint/backup stress under Windows Defender or other EDR;
- cross-machine latency, memory, executable-size, or 10k/100k chunk SLOs;
- network-filesystem support, which the architecture disallows.

`os.Exit` is an abrupt process termination with cleanup intentionally bypassed,
but the operating system still owns handle teardown. It must not be described
as a physical power-loss test. Likewise, `PRAGMA max_page_count` is a
deterministic SQLite capacity limit, not a fabricated claim that a physical
volume was filled.

Those remaining ADR 0010 items require release-machine stress/fault campaigns
and separate versioned reports before the complete driver qualification gate
can be marked `PASS`.
