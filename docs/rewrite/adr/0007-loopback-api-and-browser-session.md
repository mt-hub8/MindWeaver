# ADR 0007: Loopback API and browser session

- Status: Accepted
- Date: 2026-08-23

## Decision

The embedded UI consumes the versioned `/api/v1` HTTP API; it has no privileged
database or filesystem path. A 256-bit, one-use, short-lived bootstrap code is
placed in the URL fragment and exchanged for an in-memory session cookie. Every
unsafe request requires a session, CSRF token, exact Host, exact loopback Origin,
and non-cross-site fetch metadata.

Writes use persisted idempotency keys and mutable roots use integer versions with
ETag/If-Match. Errors use the versioned Problem contract and stable codes. SSE is
at least once, resumable by opaque event ID, bounded, and content-free outside
answer-run provisional streams.

## Consequences and verification

- No CORS grant; no external bootstrap resources; CSP forbids framing and remote
  content; HTML is `no-store`; hashed assets may be immutable.
- Session, CSRF, bootstrap, credential, prompt, source, and provider payload data
  are excluded from logs and global events.
- DNS rebinding Host attacks, CSRF, XSS, stale UI build, expired cursor, duplicate
  JSON key, oversized request, and multi-tab version races require tests.
