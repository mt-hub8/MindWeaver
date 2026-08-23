# MindWeaver HTTP API v1

`openapi.json` is the canonical, versioned OpenAPI 3.1 contract. JSON was chosen
so the contract can be parsed and checked with the Go and Python standard
libraries in an offline checkout.

Compatibility rules:

- Existing paths, operation IDs, response status codes, enum values, and required
  fields are stable throughout API v1.
- Additive optional fields and new paths may be introduced without a major API
  version change. Clients must ignore unknown response fields even where schemas
  document the currently emitted closed shape.
- Breaking changes require `/api/v2` and a separate contract directory.
- Every error response uses `application/problem+json` and the `Problem` schema.
- `retryable` is determined by stable error semantics, not by HTTP status alone;
  optional `userAction` and `retryAfter` fields guide client recovery.
- Browser writes require both `X-CSRF-Token` and `Idempotency-Key`. Updates to an
  existing resource additionally require a strong `If-Match` ETag.
- The bootstrap token is accepted only in the JSON request body. It must never be
  placed in a URL, cookie, command-line argument, or log field.
- SSE reconnects use `Last-Event-ID`; an unavailable cursor is an explicit `409`.

This contract intentionally describes protocol behavior, not storage choices.
