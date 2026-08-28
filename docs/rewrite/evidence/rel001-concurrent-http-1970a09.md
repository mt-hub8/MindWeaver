# REL-001 bounded concurrent HTTP linearization

Status: **NOT_IMPLEMENTED**

- Test sequence:
  `6d2e8b5b51f323f9c28ab735d1dda79ce3b61952` ->
  `c348097edc85429d8582387b12049d84abc66764` ->
  `15a9158fdbd44546d99f8d99bd8817eb2ebbfb4a` ->
  `1970a09447f610b4158995975ef89ff3f783dd4f`.
- Exact integration verification:
  `67978cc1f465ae94589a57c1bde50953f668a221`.
- Recorded: 2026-08-28 (Asia/Shanghai).
- Production changes: none.
- Executable sources:
  - `v2/internal/app/rel001_concurrent_http_campaign_test.go`, Git blob
    `1d4a22b00d4f3fac6353720a31b4420af4aea609`, 45,077 bytes and 1,141
    lines;
  - `v2/internal/app/rel001_lifecycle_concurrent_http_test.go`, Git blob
    `e820bfbdd76e57ed2ed3b688a1ff4c06805c5be6`, 34,458 bytes and 784
    lines.

This addendum records a bounded concurrent developer signal for the existing
CORE HTTP surface. It does not promote the conjunctive `REL-001` row.

## Concurrent replay and conflict boundary

The test starts the real `App`, ephemeral loopback listener, bootstrap/session
exchange and CSRF-protected HTTP handlers over a fresh Vault. An
`Expect: 100-continue` body barrier requires all eight upload handlers to enter
the server and all eight request bodies to reach EOF before any participant is
released. This is a real overlapping handler wave rather than a sequence of
direct handler calls.

Three scenarios are frozen:

1. Eight uploads using one idempotency key and one body produce exactly one
   `202 created:true` identity. Other participants may observe the exact replay
   or the bounded retryable busy response. Terminal replay and restart replay
   preserve the one document, revision and job.
2. Eight uploads alternate two bodies behind the same key. Exactly one body is
   admitted; the other remains a stable conflict. The losing publication may
   leave only its declared GC candidate. The first restart sweeps that candidate;
   replaying the losing body then exercises the same conflict and queues a fresh
   candidate, which the second restart sweeps before proving one winner object
   and no residue.
3. Eight Ask requests use one conversation, question, scope and key. The real
   RAG service and literal-loopback Ollama client invoke the provider exactly
   once. The admitted Answer converges from pending to one terminal durable
   identity, and every exact replay before and after restart preserves it.

The `created` field is decoded presence-aware, so a missing field cannot be
confused with `false`. Every response class, stable Problem code and returned
identity is checked before storage convergence is accepted.

## Concurrent lifecycle boundary

Four two-handler waves qualify distinct ownership and reference races:

1. Two membership additions use the same collection revision. One update wins
   with a strictly newer revision and one receives a revision conflict. Winner
   replay is unchanged and loser replay remains a conflict before restart; the
   restart then preserves the exact winner membership, revision and raw state.
2. Membership addition overlaps document trash. The final state must match one
   of the two legal serial histories; the trashed document is excluded from
   global and collection search in either case.
3. Membership removal overlaps purge of the same trashed document. Both legal
   `changed` values are accepted only when the final collection revision,
   response, replay, restart and raw database agree. The document graph, Blob,
   candidate, staging and purge residue must all be absent.
4. A same-content upload pins the shared object before reading its body while a
   purge overlaps it. The mechanical pin-first barrier requires the retained
   result (`deleted=0`, `retained=1`, `allRemoved=false`); the new upload has a
   distinct graph, exact replay and one shared object after restart.

## Independent convergence oracles

The scenarios do not infer correctness from HTTP status alone. They reopen the
Vault and independently verify:

- SQLite integrity, foreign keys, FTS5 external-content integrity and canonical
  consistency;
- exact counts and identities across the current 19 logical tables;
- idempotency request/body ownership, job attempt and Answer provenance;
- document/lifecycle/collection revisions and membership;
- global and scoped search projections;
- the exact regular-file `objects/sha256` content-address subtree, bytes and
  SHA-256;
- an empty Blob staging directory plus zero GC candidates and purge residue.

The original four commits were independently reviewed with P0=0/P1=0. Focused
replay and lifecycle waves passed with `-count=20`. At exact integration commit
`67978cc`, ordinary offline `scripts/ci.ps1` and tracked-only
`scripts/verify-standalone.ps1` both passed the complete module test, vet and
dual-PE build gates.

## Deliberate non-claims

These are deterministic bounded waves on one Windows host. They do not prove:

- a `go test -race` lane or arbitrary scheduler interleavings;
- every pair or longer sequence of CORE mutations;
- randomized concurrent multi-wave execution or process termination;
- physical-media, power-loss or kernel fault behavior;
- accepted 10k/100k performance, capacity, RSS or latency budgets;
- a 24-48 hour supported-machine soak or signed release-machine evidence.

The repository therefore has materially stronger concurrent linearization and
replay evidence, while the release-scale race, physical-fault, performance,
long-duration sequence, kill and soak facets remain open. `REL-001` remains
`NOT_IMPLEMENTED`.

## Reproduction

From `v2/` with the repository-selected Go 1.27.0 toolchain and frozen offline
vendor environment:

```powershell
go test ./internal/app -run '^TestREL001ConcurrentHTTPReplay$' -count=20
go test ./internal/app -run '^TestREL001ConcurrentHTTPLifecycle$' -count=20
go vet ./internal/app
scripts/ci.ps1 -Go '<pinned-go1.27.0>\bin\go.exe'
scripts/verify-standalone.ps1 -Go '<pinned-go1.27.0>\bin\go.exe'
```
