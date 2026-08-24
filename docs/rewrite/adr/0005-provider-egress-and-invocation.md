# ADR 0005: Provider egress and invocation

- Status: Superseded in part by ADR 0012 and narrowed by ADR 0013
- Date: 2026-08-23

## Decision

Provider configuration is immutable and versioned. Effective capability is the
intersection of user declaration, fresh probe evidence, and MindWeaver support.
ADR 0013 narrows v1 to a non-secret loopback Ollama endpoint, model, and timeout;
there is no v1 credential store or cloud-provider surface. A later cloud slice
must separately qualify credential persistence, rotation, loss, migration, and
authority binding before it can restore the broader rules below.

V1 accepts only a fixed literal IPv4 or IPv6 loopback address. It rejects DNS
names, including `localhost`, and therefore performs no DNS resolution. The
broader resolution-authorization rules below apply only to a future accepted
non-loopback provider slice.

Every external request passes a default-deny egress decision for exact provider,
capability, scheme, host, port, and post-resolution address class. The HTTP
transport dials one approved address directly, preserves the authorized TLS host,
does not re-resolve DNS, disables ambient proxy use, and repeats authorization
for every redirect. V1 attaches no credential. A future credential may be
attached only after the final authority is authorized.

The durable invocation protocol is:

```text
PREPARED -> SENT -> RECEIVED -> COMMITTED
PREPARED -> FAILED_DEFINITE / CANCELLED
SENT     -> RECEIVED / FAILED_DEFINITE / OUTCOME_UNCERTAIN
```

`RECEIVED` requires a durable checksummed result artifact. Non-idempotent calls
left in `SENT` become `OUTCOME_UNCERTAIN` and are not automatically replayed.

## Consequences and verification

- Replay policy is explicit: `SAFE`, `UNSAFE`, or `PROVIDER_IDEMPOTENT`.
- Invocation records bind provider config version/fingerprint, model identity,
  request hash, purpose, and content-free usage/error metadata.
- DNS rebinding, mixed DNS answers, redirects, proxy variables, metadata ranges,
  cancellation, partial stream, timeout, crash, and duplicate-cost tests gate
  release.
