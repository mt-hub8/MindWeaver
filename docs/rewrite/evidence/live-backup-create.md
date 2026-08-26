# Live backup create boundary

Status: implemented candidate, `BKP-001` remains `BLOCKED`. This evidence does
not qualify a release.

## Product boundary

The loopback workbench exposes only three live backup operations:

- accept one no-replace backup create with session, CSRF, and an
  `Idempotency-Key`;
- read bounded process-local status by opaque operation ID;
- request cancellation of an accepted operation.

The HTTP request does not own the long-running copy context. App shutdown stops
new ingress, cancels and waits for the owned backup runtime, and only then
closes the coordinator, SQLite store, and Vault. Recovery verify and restore
remain startup-only commands and are not registered as live HTTP routes.

Every accepted key is committed to the exact destination for the process
lifetime, up to a fixed 4,096-operation bound. Reusing a key for another target,
exhausting the bound, overlapping work, or quiescing fails closed. Operation
IDs include a random process nonce and expose no destination. Status and error
responses contain stable phases, counts, byte totals, and failure codes only.

An already existing, fully verified backup is reported as
`BACKUP_EXISTING_VERIFIED` with `needs_attention`; it is never claimed as a
snapshot produced by the new request. Backup packages contain plaintext SQLite
and source objects. The UI and OpenAPI state that confidentiality depends on
the user's account, media access controls, and disk-encryption policy.

## Filesystem and interruption invariants

Create rechecks a retained fixed-local destination-parent capability before
its first write. Windows validates owner/DACL, rejects reparse points, remote or
non-fixed media, per-directory case-sensitive namespaces, managed staging
ancestors, and unsafe leaf aliases. Every validated ancestor is retained with
no delete sharing while later path-only operations run. Exact residue matching
uses Windows ordinal case-insensitive comparison, not Go Unicode simple-fold.

Publication uses no-replace rename plus a versioned receipt/binding pair. The
receipt records enough identity to distinguish staging, publication uncertain,
and publication-cleanup-pending states. Cleanup and confirmation require exact
identity/CAS matches; conflict, truncation, another destination, active residue,
or ambiguous publication is retained for attention. A forced-exit test kills a
child after the published receipt has been durably removed but before its
binding is removed, then proves restart classifies cleanup-pending, verifies the
published backup, preserves it, and removes only the exact evidence.

Confirmation is a point-in-time integrity statement linearized at the final
path, manifest commitment, and complete tree recheck. Deleting the exact redo
evidence afterward does not claim that another process under the same Windows
account cannot modify the plaintext package after that point.

The UI keeps the destination immutable for an accepted or response-uncertain
attempt, reuses the same idempotency key, serializes polling, and ignores stale
poll/cancel responses. It cannot silently turn an edited destination into a new
request under an old key.

## Executed evidence

The focused Windows suites cover concurrent-write snapshots, no-overwrite,
disk-full classification, cancellation, bounded history, shutdown ordering,
receipt/binding interruption windows, replacement races, junctions, ancestor
rename attempts, case variants, and content/path canaries. They are run with
the repository-pinned Go 1.27 toolchain, offline module resolution, and CGO
disabled. The final integration gate must repeat:

```text
go test ./internal/backup ./internal/app ./internal/webui ./openapi/v1
go test ./...
go vet ./...
scripts/ci.ps1 -Go <go1.27>
scripts/verify-standalone.ps1 -Go <go1.27>
```

## Open blocker

The retained ancestor chain closes junction and ordinary ancestor-rename
egress, but the public Vault/Store boundary still represents roots as `X:\...`
strings. A same-session `DefineDosDevice` remap can therefore change the DOS
drive mapping between a retained check and a later absolute-path-only Vault,
SQLite, durability, or rename call. Rechecking the mapping would leave another
check/use race and is not accepted as a fix.

Closing this boundary requires a separate capability slice: derive a canonical
local kernel path from the retained volume handle, carry kernel and display
paths separately through Vault/Store/blob ownership, use handle-relative rename
where available, and qualify SQLite plus every Windows path consumer. Until
that is implemented and the clean-machine restore/release rehearsal passes,
`BKP-001` remains `BLOCKED` rather than `PASS`.
