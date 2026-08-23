# ADR 0004: Durable jobs, events, and recovery

- Status: Accepted
- Date: 2026-08-23

## Decision

All asynchronous work uses one durable Job/Attempt protocol in SQLite. Claim is
one compare-and-swap transaction that advances attempt and fencing token, writes
the lease and Attempt, and appends an event. Checkpoint and completion require
matching job ID, writer epoch, lease owner, fencing token, and target operation
epoch. Late workers cannot commit.

Business state and its domain event commit in the same transaction. The browser
event stream is an at-least-once projection ordered by a Vault-wide sequence.
External I/O never occurs inside a database transaction. Retry is bounded and
classified; cancellation request is distinct from terminal cancellation.

## Consequences and verification

- Required states include `PENDING`, `RUNNING`, `RETRY_WAIT`, `SUCCEEDED`,
  `FAILED`, `CANCELLED`, and `NEEDS_ATTENTION`.
- Duplicate commands return the original operation through a persisted
  idempotency record committed with the business root.
- Expired leases become abandoned Attempts; recovery decides retry from the job
  kind and external invocation state, never from a caller-controlled boolean.
- Crash, lease race, cancellation race, poison-job, cursor expiry, and slow-event
  consumer tests are release gates.
