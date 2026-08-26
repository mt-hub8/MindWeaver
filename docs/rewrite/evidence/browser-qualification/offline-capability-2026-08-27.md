# UI-001 / UI-002 offline browser capability decision

## Decision

At `2026-08-27T01:44:41+08:00`, Windows amd64 qualification is **BLOCKED**
with stable code `BROWSER_ARTIFACT_NOT_APPROVED`. No real UI-001/UI-002
scenario is claimed as run or passed.

The machine has an ambient system Edge `151.0.4129.101` whose observed SHA-256
is `24f626e48dae3574b4d59adc8f23722f890fd0a64f78c772384b783b96bcf1a0`.
It is an automatically updated installation, not a repository-approved offline
artifact. No `msedgedriver`, `chromedriver`, or `geckodriver` executable was
found in PATH, the configured Codex runtime, or the browser plugin. The two
standard Playwright browser cache roots do not exist.

The configured Codex Node runtime contains Playwright/Playwright-core 1.62.1,
but no browser executable. An ambient automation library is neither a pinned
browser artifact nor a release qualification dependency. No browser or driver
was downloaded, copied, or approved during this review.

The content-free machine-readable observation is
[`offline-capability-2026-08-27.json`](./offline-capability-2026-08-27.json).
It intentionally contains no executable path, profile, Vault path, cookie,
CSRF value, environment variable, prompt, source text, or process output.

## Implemented fail-closed boundary

- `v2/tests/browser/approval.v1.json` is the repository-owned approval record.
  Its artifact set is empty. The parser permits at most one exact Windows/amd64
  browser/driver/mindweaver tuple and rejects duplicate keys, unknown/trailing
  input, unsafe leaves, ambiguous names, oversized input, and invalid identity.
- `v2/tests/browser` freezes all 13 required UI-001/UI-002 scenario IDs and an
  opaque evidence report. A blocked prerequisite marks every scenario
  `NOT_RUN/PREREQUISITE_BLOCKED` and cleanup `NOT_STARTED`.
- The approved path retains opened handles for all three artifacts, verifies
  file identity/size/SHA-256 before and after orchestration, rejects extra
  bundle entries, symbolic links, and hard links, and accepts only a literal `127.0.0.1` WebDriver
  endpoint. The package-owned protocol bounds time and a three-process tree,
  aggregates all 13 scenario screenshot/trace hashes, deletes the WebDriver
  session, and emits a content-free cleanup receipt hash.
- The repository-controlled fake WebDriver/process harness exercises that
  protocol and its cleanup/failure seams only. Its success tuple is permanently
  `BLOCKED/CONTROLLED_HARNESS_NOT_QUALIFIED`; it has no package-owned real-run
  proof and therefore cannot emit PASS or close UI-001/UI-002.
- `v2/tests/browser/runner` writes a bounded JSON report without overwriting an
  existing target. It publishes a fully written sibling atomically and emits
  only a stable status/code line.
- `v2/scripts/test-browser.ps1` forces `GOTOOLCHAIN=local`, `GOPROXY=off`,
  `GOSUMDB=off`, `CGO_ENABLED=0`, Windows amd64, and a trimpath/no-VCS build of
  the test-only runner. Normal evidence requires a clean worktree and a new
  report target outside that worktree. It invokes approval before any product
  process. With the current empty approval it must exit 3 and cannot build or
  start `mindweaver.exe`, fake Ollama, a driver, or a browser.

The PowerShell `-SelfTest` takes a before/after identity snapshot of relevant
process names, verifies the 13-scenario BLOCKED report, proves the report does
not contain its temporary path, and removes its validated private temporary
tree. Existing browser/model processes are neither inspected beyond identity
nor stopped.

## Qualification matrix

| Gate | Current evidence | Status |
| --- | --- | --- |
| Approved, fixed offline browser | Empty approval record; ambient Edge explicitly rejected | `BLOCKED` |
| Matching approved driver | No driver artifact present | `BLOCKED` |
| Fully offline runner | Offline build flags and script self-test | `PASS` |
| Approval before product process | Process-boundary call count is zero when blocked | `PASS` |
| Content-free bounded report | Go and script tests | `PASS` |
| Real `mindweaver.exe` + fresh Vault | Not started because prerequisite failed | `NOT_RUN` |
| Literal-loopback fake Ollama | Not started because prerequisite failed | `NOT_RUN` |
| Controlled protocol scenarios | 13 fake WebDriver flows plus fault injection; explicitly not qualification | `NOT_QUALIFIED` |
| Independent real browser scenarios | Empty approval prevents any product process | `NOT_RUN` |

## P0/P1 review

- P0: none in the fail-closed path. The current public runner cannot emit PASS
  and cannot cross the process boundary.
- P1: a versioned, redistribution-approved browser plus matching driver bundle
  is external evidence still missing.
- P1: a real Windows Job/ACL-backed launcher, fresh-Vault lifecycle, actual
  browser interaction implementation, and OS-verified descendant cleanup are
  not implemented. Adding them would expand the production/release boundary,
  so this slice stops at the reviewable protocol boundary.

System Edge and Codex browser-control capabilities must not be substituted for
that evidence. UI-001 and UI-002 remain BLOCKED rather than degraded to DOM,
handler, mocked-fetch, or string-test PASS.

## Executed offline checks

```text
go1.27.0 test ./tests/browser/... -count=10                       PASS
go1.27.0 vet ./tests/browser/...                                  PASS
scripts/test-browser.ps1 -SelfTest (10 consecutive executions)   PASS
report-inside-source rejection/no-write                           PASS
runner dependency closure: 84 packages, forbidden production 0   PASS
scripts/verify-standalone.ps1                                     PASS
```

The standalone run executed the full offline module test gate, including the
new contract and runner packages. None of these checks contacted a model,
browser download service, package proxy, MySQL source, or user Vault.
