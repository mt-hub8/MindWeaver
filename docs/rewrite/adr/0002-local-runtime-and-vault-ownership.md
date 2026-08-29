# ADR 0002: Local runtime and Vault ownership

- Status: Accepted
- Date: 2026-08-23

## Decision

MindWeaver v2 is a local, single-user workbench. One process opens one Vault and
one OS-backed exclusive lock establishes the sole writer. A PID file is
diagnostic only. The HTTP listener binds an ephemeral port on exact loopback;
LAN and public binding are outside v1.

Startup is staged and durable: lock, open, compatibility check, migrate,
reconcile, start workers, then become ready. Ordinary writes are unavailable
during recovery. Shutdown stops new work, checkpoints bounded in-flight work,
invalidates sessions, and releases the Vault lock. Correctness never depends on
a graceful shutdown.

## Consequences and verification

- A second process requests the existing process to open a browser and never
  steals a live lock.
- Newer schema or writer-version Vaults fail closed.
- Forced termination at every startup and shutdown checkpoint must recover to a
  valid state or explicit read-only/`NEEDS_ATTENTION` state.
- Windows path identity, symlink/reparse points, UNC/sync/removable media policy,
  sleep/resume, and lock release require platform acceptance tests.
