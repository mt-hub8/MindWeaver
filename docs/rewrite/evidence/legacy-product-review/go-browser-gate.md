# Go browser qualification gate

## Decision

The embedded-asset Go tests remain fast contract tests; they are not browser
evidence and cannot close `UI-001` or `UI-002`. A release PASS requires the
actual Windows `mindweaver.exe`, an actual browser process, the loopback HTTP
session boundary, a temporary empty Vault, and a real loopback fake Ollama
server in one test run. `page.setContent`, direct database seeding, handler-only
servers, mocked `fetch`, and Java/Python processes are disallowed.

The browser runner and browser binary must be version-pinned, checksummed, and
available offline. If the pinned binary is missing, the gate reports `BLOCKED`;
it must not download a browser or silently replace the run with DOM/string
tests.

## Planned file boundary

The gate is intentionally not implemented in this evidence commit. When the
pinned browser artifact is approved, implementation is limited to:

```text
v2/tests/browser/**                 # browser scenarios and bounded fixtures
v2/scripts/test-browser.ps1         # offline launcher and artifact verifier
v2/scripts/ci.ps1                   # required Windows release invocation
v2/testdata/browser/**              # non-secret, synthetic TXT/Markdown corpus
```

Production handlers, Store schema, RAG/provider protocols, Java, Python,
Agent, vector, and migration packages are not browser-test scaffolding.

## Required execution topology

1. Build `mindweaver.exe` with the same flags used for the release candidate.
2. Create an owner-only temporary Vault and launch the executable as a child.
3. Read only its bounded readiness/bootstrap handoff; do not manufacture a
   cookie or CSRF token in the test.
4. Launch the pinned browser with a new temporary profile, proxy disabled, and
   no inherited extensions or user credentials.
5. Navigate through the public root and consume the one-use bootstrap flow.
6. Run a separate literal-loopback fake Ollama HTTP process that exercises the
   same transport used by production Ask. It must support success, malformed
   citation, no-hit, slow response, disconnect-after-request, and cancellation.
7. Shut down and reopen the real executable and browser where the scenario
   requires restart evidence. Inspect the Vault only after both processes close.

## Required scenarios

The release gate fails unless it proves all of the following through visible UI
state and the underlying public API behavior:

- first-run bootstrap is one-use; refresh obtains a valid session and a stale
  CSRF token fails;
- no-model mode still uploads, indexes, catalogs, scopes, and searches a
  synthetic TXT/Markdown document;
- Ollama configuration accepts only literal loopback, probe failure does not
  activate configuration, and no credential UI exists;
- Ask persists a completed answer whose displayed citation positions resolve
  only to the exact displayed sources used by that answer;
- no-hit and malformed/foreign citation responses show an explicit limitation
  and never use “verified”, “fact checked”, or equivalent wording;
- timeout/disconnect/cancel shows a durable failed or outcome-uncertain result
  and never automatically replays a provider call;
- reload and process restart resume pending-answer polling by the same answer
  ID without issuing another Ask;
- two tabs exercise stale revision and idempotent retry behavior without
  duplicate conversations or answers;
- keyboard-only navigation, visible focus, focus restoration after errors,
  200% zoom/reflow, forced colors, reduced motion, and Chinese IME composition
  preserve operability and do not submit partial input;
- browser console errors, unhandled promise rejections, unexpected external
  requests, and non-loopback connections fail the run.

## Evidence and leakage rules

The runner emits a machine-readable result with executable/browser hashes,
scenario IDs, timestamps, and pass/fail codes. Screenshots and traces are
bounded and use only the synthetic corpus. Reports must not include Vault paths,
cookies, CSRF values, prompts, source text, model output beyond fixed canaries,
or environment variables. A failed cleanup is reported separately and never
turns a failed scenario into PASS.

Until this runner is committed and passes on the final release closure,
`UI-001` and `UI-002` remain `NOT_IMPLEMENTED`. The current wording fix closes
only the citation-assurance overclaim.
