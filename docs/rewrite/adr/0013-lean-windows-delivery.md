# ADR 0013: Lean Windows delivery and explicit plaintext boundary

- Status: Accepted
- Date: 2026-08-24
- Supersedes: the first-release cloud-provider/credential-store requirement in
  ADR 0005, and the Vault master-key/CredMan/DPAPI recovery requirement,
  named-pipe activation protocol, and built-in updater/turnstile in ADR 0009

## Context

ADR 0009 combined the proven first-release boundaries (a handle-identified,
exclusively locked Vault, a secured loopback browser session, and a signed
per-user MSI) with three systems that have no current product consumer:
application-level Vault encryption and key recovery, second-process activation,
and a self-updater. Implementing dormant security or update machinery would add
new irreversible state and recovery promises without closing a user workflow.

The first release is a single-user local workbench. It stores source bytes and
SQLite state locally, uses optional loopback Ollama without a credential, and
can tell a second launcher that the Vault is already owned. Installation and
upgrade can remain an explicit operating-system package operation.

## Decision

### Boundaries retained from ADR 0009

- A writable Vault is supported only on a validated local fixed NTFS volume,
  is identified through retained handles, and is owned through one exclusive
  process-lifetime lock.
- The browser server binds exact loopback and retains the one-use bootstrap,
  session, Host, Origin, CSRF, and CSP boundaries from ADR 0007 and ADR 0009.
- The Windows release artifact is a signed per-user MSI. Uninstall preserves
  user Vaults by default. Clean non-admin installation, upgrade from two real
  prior packages, rollback, uninstall, signature verification, and Vault
  preservation remain release gates.

### No dormant key hierarchy

The v1 Vault is **not application-encrypted**. Its SQLite database, source blobs,
and backup packages are plaintext files protected by the user's Windows account,
filesystem permissions, and chosen storage. Product UI, CLI documentation, and
backup workflows must disclose this boundary; no code may imply that a backup is
encrypted.

There is no Vault master key, Credential Manager record, DPAPI fallback, recovery
package, or `RECOVERY_REQUIRED` key state in v1. Optional loopback Ollama has no
stored secret. A future cloud credential or application-encryption feature must
arrive through a new ADR with an actual consumer, versioned persistence,
rotation, backup, loss, and migration tests. It must not silently reinterpret an
existing plaintext Vault.

### No second-process activation protocol

A second process reports the stable `VAULT_BUSY` outcome and exits. It does not
open a named pipe, trust a PID record, or forward a route to the owner. A future
desktop-shell consumer may qualify activation separately; until then the pipe,
DACL, protocol, and activation endpoint are absent rather than dormant.

### No built-in updater

V1 does not download, stage, or install its own update and has no updater
turnstile. Upgrade is a deliberate invocation of an already obtained, verified
MSI. Build/release automation may produce a canonical signed artifact manifest
and release index, but the running application does not accept a manifest as
authority to replace itself.

Release tooling must not download WiX, accept a tool EULA, install a certificate,
or create a signing identity on the user's behalf. Missing approved offline
tools, licenses, certificates, historical MSIs, or clean-machine evidence is a
machine-readable `BLOCKED`, never a synthetic pass.

## Consequences

- `KEYSTORE_UNAVAILABLE`, key-loss recovery, named-pipe activation, built-in
  updater state, and application-orchestrated self-update rollback are not CORE
  runtime states. Windows Installer rollback remains a release gate.
- `BROWSER_OPEN_FAILED` and `VAULT_BUSY` keep direct manual recovery paths: copy
  the printed one-use URL, or close the process that already owns the Vault.
- Backup/restore safety remains a CORE capability, but it guarantees integrity,
  identity, and recoverability—not confidentiality.
- No package, table, config field, feature flag, or placeholder is retained for
  a deferred key store, activation pipe, or updater.

## Verification

Completion requires real child-process Vault exclusion, forced-exit/reopen,
loopback session attack tests, plaintext disclosure tests, and the signed MSI
clean-machine matrix. It does not require CredMan, DPAPI, named-pipe, or
self-update tests unless a later accepted ADR promotes those capabilities.
