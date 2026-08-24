# ADR 0009: Windows Vault runtime

- Status: Accepted
- Date: 2026-08-23
- Narrowed by: ADR 0013 removes the unconsumed key hierarchy, named-pipe
  activation protocol, and built-in updater from the first-release CORE

## Context

Windows 10/11 x64 is the first Tier-1 platform. A path string, PID file, named
mutex, or SQLite lock by itself is not a sufficient identity and ownership
boundary for a user-owned Vault. The runtime must also remain safe across path
aliases, reparse points, a second process, forced termination, cloud-synced or
removable media, browser bootstrap, credential loss, and offline updates.

## Decision

### Vault identity and ownership

The writable Vault is supported only on a local fixed NTFS volume in v1. The
runtime rejects UNC and mapped drives, Cloud Files sync roots, removable or
hot-plug media, device paths, alternate data streams, and any unresolved media
classification. Such locations may be import sources or backup destinations,
but never an active Vault.

The runtime opens the directory and derives its identity from
`GetFileInformationByHandleEx(FileIdInfo)` plus the volume identity. It uses
`GetFinalPathNameByHandleW` only as a canonical display path. Equality and lock
ownership never depend on lower-cased path strings. It inspects each controlled
component for name-surrogate reparse points and, after validation, retains an
`os.Root`; internal storage accepts only validated relative paths.

The sole-writer authority is a dedicated local lock file opened with
`CreateFileW` and held for the Vault lifetime by an exclusive, immediate
`LockFileEx` byte-range lock. A PID/owner JSON record is diagnostic and supplies
the activation endpoint, but can never authorize lock theft. SQLite retains its
own locking and recovery responsibilities.

The writable database composition root requires an acquired, package-private
Vault lease. It is impossible to construct a writable Store merely from a path.

### Secrets, browser, and activation

Each Vault has a random 32-byte master key. The primary Windows key store is a
Generic Credential scoped to the current user and machine. User-scope DPAPI is
the fallback; machine-scope DPAPI and plaintext fallbacks are forbidden. A
missing master key enters `RECOVERY_REQUIRED` and never silently creates a new
identity. A separately exported recovery package is required for profile or
machine loss.

The HTTP server binds `tcp4` on exact `127.0.0.1:0`. `ShellExecuteW` runs on a
dedicated COM STA thread. Browser bootstrap uses a 256-bit, one-use, short-lived
fragment token; the page exchanges it for an HttpOnly, SameSite=Strict session.
The server validates Host, Origin, and CSRF and never enables CORS.

A second process that observes `VAULT_BUSY` may contact the owner only through a
random named pipe recorded by the lock owner. The pipe has a current-logon-SID
DACL, rejects remote clients, applies size and deadline limits, and accepts only
versioned `Activate` or allow-listed relative-route messages. A fixed pipe name
or default DACL is forbidden.

### Install and update

The initial package is a signed per-user MSI. Program files and user Vault data
are separate; uninstall preserves Vaults by default. An offline update requires
an Ed25519-signed versioned manifest and per-file SHA-256/length/path checks.
Authenticode is a second, platform trust layer rather than the application
update authority.

Update follows this durable sequence:

```text
IMPORTED -> VERIFIED -> STAGED -> QUIESCING -> BACKED_UP
         -> APP_EXITED -> INSTALLING -> BOOT_CHECK -> COMMITTED
```

The updater uses a second byte-range turnstile to prevent a new app process
starting between quiescence and replacement. It never overwrites a running
executable and never relies on delayed-until-reboot replacement for correctness.

## Stable failures

At minimum the adapter maps native failures to:

- `VAULT_BUSY`
- `VAULT_RECOVERY_REQUIRED`
- `VAULT_PATH_UNSAFE`
- `VAULT_REMOTE_UNSUPPORTED`
- `VAULT_CLOUD_SYNC_UNSUPPORTED`
- `VAULT_MEDIA_UNSAFE`
- `KEYSTORE_UNAVAILABLE`
- `RECOVERY_REQUIRED`
- `BROWSER_OPEN_FAILED`
- `ACTIVATION_UNAVAILABLE`
- `UPDATE_SIGNATURE_INVALID`
- `UPDATE_DOWNGRADE_BLOCKED`
- `UPDATE_ROLLBACK_REQUIRED`

Win32 codes and HRESULT values remain diagnostic details, not public protocol
codes.

## Verification

PR tests use real child processes for lock exclusion and forced termination.
Nightly Windows desktop VMs cover multiple local users, path aliases, reparse
replacement races, OneDrive consumer/business, SMB, removable media, CredMan,
DPAPI, named-pipe ACLs, sleep/resume, and bootstrap attacks. The release gate
uses a clean non-admin, offline VM for install, update, rollback, and uninstall,
and verifies every final signature and retained Vault.

The implementation and tests must use the primary platform contracts, not a
second best-effort Windows-only ownership model.

## Authoritative references

- [LockFileEx](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-lockfileex)
- [GetFinalPathNameByHandleW](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-getfinalpathnamebyhandlew)
- [FILE_ID_INFO](https://learn.microsoft.com/en-us/windows/win32/api/winbase/ns-winbase-file_id_info)
- [Cloud Files sync-root inspection](https://learn.microsoft.com/en-us/windows/win32/api/cfapi/nf-cfapi-cfgetsyncrootinfobyhandle)
- [Go `os.Root`](https://pkg.go.dev/os#Root)
- [Credential Manager `CredWriteW`](https://learn.microsoft.com/en-us/windows/win32/api/wincred/nf-wincred-credwritew)
- [User-scope DPAPI](https://learn.microsoft.com/en-us/windows/win32/api/dpapi/nf-dpapi-cryptprotectdata)
- [Named-pipe security](https://learn.microsoft.com/en-us/windows/win32/ipc/named-pipe-security-and-access-rights)
- [Windows Installer per-user package authoring](https://learn.microsoft.com/en-us/windows/win32/msi/single-package-authoring)
- [WinVerifyTrust](https://learn.microsoft.com/en-us/windows/win32/api/wintrust/nf-wintrust-winverifytrust)
