# Current browser qualification boundary

Status: **SEC-001, UI-001, and UI-002 remain BLOCKED**

- Windows sandbox rewrite:
  `69f263c4e19a29c34f66e1102ed40cf4cd89b822`
- Bounded helper and assigned-process cleanup:
  `ed7e2016491b0e7a0048ad3a2e28a54c2ae561f4`
- Current scenario/report contract:
  `2d9d07e523ee94d01eb58f2896aa9d48d188d2f8`
- Authenticated-report hardening:
  `c384a40be21d41a2aec85f9a69b7afcc1c5e5aa1`
- Exact case-sensitive Go JSON-key hardening:
  `9ae35517fe4713a5423a87bcbf88afdd93a73398`
- Recorded: 2026-08-27 (Asia/Shanghai)

Earlier 13-scenario reports remain immutable historical observations. This
document supersedes only their current scenario count and blocker wording; it
does not rewrite their source baseline, artifact inventory, or measurement.

## Closed internal contract

The checked-in approval is empty. The public runner therefore returns stable
`BLOCKED/BROWSER_ARTIFACT_NOT_APPROVED`, marks every scenario `NOT_RUN`, and
does not open an artifact or cross the product-process boundary.

If a future repository review supplies one exact Windows/amd64 artifact tuple,
the admission contract binds browser, driver, and MindWeaver role, leaf name,
byte size, SHA-256, and version. Retained fixed-local, owner-only protected
artifact capabilities and Windows Job Object tests cover suspended start,
assign-before-resume, image and membership revalidation, descendant accounting,
termination, OS-observed zero active processes, and bounded handle cleanup.

Report schema v2 is fail closed:

- duplicate keys, unknown fields, wrong JSON types, invalid UTF-8, trailing
  values, oversized input, and incorrect scenario order are rejected;
- PASS requires the exact approved artifact tuple, policy hash, root and
  descendant lineage hashes, closed WebDriver session, post-run artifact
  revalidation, bounded OS process totals, zero active processes, and all
  screenshot/trace hashes and sizes;
- `cleanupReceiptSha256` is recomputed over the same canonical JSON bytes in Go
  and PowerShell, with a shared fixed vector and mutation tests;
- the in-memory synthetic PASS fixture tests only the verifier. It has no path
  into public `RunQualification` or a published report;
- the controlled WebDriver harness lacks the private real proof and can emit
  only `BLOCKED/CONTROLLED_HARNESS_NOT_QUALIFIED`.

The current exact matrix contains 18 scenarios: one SEC-001 CSP case, fourteen
UI-001 cases covering bootstrap/session, CSRF, network/model configuration,
upload/search/Ask, uncertain restart, concurrency, backup, diagnostics, and
ingestion progress, plus three UI-002 keyboard/focus, visual-accessibility, and
Chinese IME cases. Startup-only recovery commands are not presented as browser
scenarios.

## Remaining external and real-run evidence

No repository-approved, redistributable browser plus matching driver tuple is
available. There is also no approved browser-family argument/profile/network
policy or real fresh-Vault orchestration with a literal-loopback fake Ollama.
Consequently, no package-owned run has produced the OS-observed lineage,
screenshots, traces, and cleanup receipt for the 18 scenarios.

Ambient auto-update Edge, a Codex/Playwright library without its approved
browser binary, the controlled fake driver, and the Windows Job/ACL primitives
are not substitutes for that evidence. They cannot close SEC-001, UI-001, or
UI-002.

## Reproduction

From `v2/` with the repository-selected Go 1.27.0 toolchain and offline vendor
environment:

```powershell
go test ./tests/browser/... -count=1
go vet ./tests/browser/...
./scripts/test-browser.ps1 -SelfTest -GoExecutable $env:MW_GO
```

The tests and vet pass. SelfTest emits only
`UI-001/UI-002 SELF_TEST_PASS BLOCKED_AS_DESIGNED`; it never publishes a PASS
report. Independent review of the authenticated-report change found P0=0 and
P1=0 within this internal contract. The three acceptance rows remain BLOCKED
until the external artifact and real-run evidence above exists.
