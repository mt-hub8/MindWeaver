# Windows registered-volume boundary

Status: implemented candidate. This evidence closes the same-session,
unprivileged `DefineDosDevice` drive-remapping boundary for Vault and backup
admission. It does not qualify `BKP-001` or a release by itself.

## Frozen scope

- Base: `57e293bb2f3d164dfda9d9840f1227dce5517f07`
- Code/test commit: `b3d2a50b85855d904580b670962cd9bf6cce7854`
- Cross-process test hardening:
  `e737387a6127fcd356178e956f67fb46564e16d7`
- Production files:
  - `v2/internal/vault/platform_windows.go`
  - `v2/internal/backup/leaf_windows.go`
- Test files:
  - `v2/internal/vault/reparse_windows_test.go`
  - `v2/internal/backup/leaf_windows_test.go`

SQLite, blob, app, OpenAPI, migration, release, and non-Windows production
files are unchanged. Existing fixed-local-media, NTFS, hot-plug, Cloud Files,
reparse-point, namespace-retention, and case-semantics checks remain in force.

## Narrow admission rule

Windows Vault and backup paths already require an ordinary drive-letter root.
After the existing DOS-device and fixed-drive checks, admission now:

1. resolves the drive root to a volume GUID with
   `GetVolumeNameForVolumeMountPointW`;
2. enumerates that volume's Mount Manager paths with
   `GetVolumePathNamesForVolumeNameW`;
3. requires the exact drive root, compared case-insensitively, to occur in the
   bounded, double-NUL-terminated result.

Enumeration errors, oversized responses, truncation, malformed termination,
and absence of the exact root all fail closed as `vault.ErrUnsafeMedia`.
A session-local direct-volume alias can otherwise look like fixed local media
to both `QueryDosDeviceW` and `GetDriveTypeW`; it is not a registered Mount
Manager path and is therefore rejected before Vault or backup writes.

This is deliberately an admission restriction, not a general canonical-path
abstraction. Once admitted, the drive letter is a system-registered volume
mount that an unprivileged same-session caller cannot shadow with a local DOS
device definition. Administrator or `LocalSystem` changes to global volume
mount state are outside this boundary and are not claimed as protected.

## Real Windows proof

The pre-implementation probe and committed regression test ran without an
elevated token. They selected an unused `Z:` only after proving it absent from
`QueryDosDeviceW`, `GetLogicalDrives`, and filesystem lookup, then created a
no-broadcast raw alias to the current fixed volume:

```text
alias              Z:
target             \Device\HarddiskVolume4
volume GUID        \\?\Volume{17a7603f-da16-40b9-9d90-5da5f72fa200}\
registered paths   D:\
alias registered   false
elevated token     false
```

The test proves the alias and source identify the same directory, then proves
all of the following:

- the ordinary registered temporary-directory path remains accepted;
- `vault.Open` rejects the alias with `ErrUnsafeMedia` before creating the
  lock, `data`, or `blobs`;
- backup namespace admission and `newDestination` reject the alias before
  creating destination state;
- cleanup uses `DDD_REMOVE_DEFINITION`, `DDD_EXACT_MATCH_ON_REMOVE`, the exact
  raw target, and no broadcast;
- after cleanup the alias is absent from `QueryDosDeviceW`, logical-drive
  enumeration, and filesystem lookup.

The cleanup verification is part of the committed test; failure is a test
failure rather than a skip. The test holds a bounded named Windows mutex from
unused-letter selection through exact deletion and final absence checks. Since
mutex ownership is thread-affine, the owning goroutine is pinned to one OS
thread for that lease. Timeout or an abandoned mutex fails the test; it never
continues into an ambiguous cleanup state.

## Executed gates

All commands used Go 1.27.0 on Windows/amd64 with `GOTOOLCHAIN=local`, offline
module resolution, readonly modules, and `CGO_ENABLED=0`.

```text
go test ./internal/vault ./internal/backup \
  -run '^TestWindowsSessionLocalDirectVolumeAliasIsRejectedBeforeVaultOrBackupWrite$' \
  -count=1 -v                                                        PASS
go test ./internal/vault ./internal/backup \
  -run 'TestWindows(RegisteredVolumePathResponseFailsClosed|SessionLocalDirectVolumeAliasIsRejectedBeforeVaultOrBackupWrite)$' \
  -count=10                                                          PASS
go test ./internal/vault ./internal/backup -count=1                 PASS
go vet ./internal/vault ./internal/backup                           PASS
4 concurrent processes, each running the real alias test -count=20 PASS
post-stress QueryDosDevice E:-Z: residue scan                       EMPTY
post-stress logical drives                                         C:\ D:\
```

Final whole-tree and standalone gates must run from the clean documentation
commit. `BKP-001` remains `BLOCKED` until the separate clean-machine
restore/release rehearsal passes.
