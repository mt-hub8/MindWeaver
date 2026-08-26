# Legacy static UI source review

## Exact frozen set

- Baseline: `0df22ddaf02c64bf73a7df12cd5fea6b52632c73`
- Machine-readable master manifest: [`legacy-static-ui.csv`](./legacy-static-ui.csv)
- Verification: `pwsh -NoProfile -File docs/rewrite/evidence/legacy-product-review/verify.ps1 -Manifest docs/rewrite/evidence/legacy-product-review/legacy-static-ui.csv`
- Exact tree: every Git blob below `src/main/resources/static`, no substring selection.
- Exact result: **47 unique files / 10,223 lines**, each with one owner, one
  `DROP` decision, one SHA-256, and complete `1..line_end` coverage.

| Unique owner | Files | Lines | Decision |
| --- | ---: | ---: | --- |
| Agent | 5 | 885 | `DROP` |
| Batch | 2 | 264 | `DROP` |
| Documents / collections / trash | 7 | 1,800 | `DROP` |
| Evaluation / knowledge health | 4 | 534 | `DROP` |
| Ingestion analytics | 2 | 378 | `DROP` |
| Memory | 2 | 315 | `DROP` |
| Notification | 2 | 119 | `DROP` |
| Provider settings | 2 | 682 | `DROP` |
| Ask / legacy RAG demo | 6 | 1,531 | `DROP` |
| Retrieval settings | 2 | 138 | `DROP` |
| General settings | 2 | 222 | `DROP` |
| Shared shell / landing / guide | 9 | 3,177 | `DROP` |
| Vector health | 2 | 178 | `DROP` |
| **Unique master total** | **47** | **10,223** | **zero exceptions** |

Static files referenced in the RAG/Provider, Agent/Memory, and
Evaluation/Batch/Health partition manifests were cross-layer dependency
evidence. They are counted only here in the repository-wide unique coverage.
The master CSV therefore closes the exact-set requirement without inflating
coverage from repeated references.

## Findings

### P0: stored collection names reach `innerHTML` without escaping

`vector-index-health.js:36-42` loads collections and concatenates each `c.name`
and `c.id` into an option string assigned to `innerHTML`. Collection names are
user-controlled product data. The same file concatenates audit issue type,
severity, and message into `innerHTML` at `68-77`, and its `setDl` helper writes
unescaped row values at `15-18`. A stored name such as an option-closing tag
with an event handler can execute script when the page loads.

This is not an isolated rendering primitive that is safe to transplant.
Several pages build HTML strings, each with a locally different `escapeHtml`
implementation; some escape only `&`, `<`, and `>`, while others omit escaping
for values they assume are enums or diagnostics. The legacy static tree has no
single typed text/attribute/URL rendering boundary.

### P0: privileged mutations have no browser security protocol

The pages issue provider-key/config changes, uploads, deletes, purge, retry,
reindex, batch cancellation, cache cleanup, and vector cleanup through direct
`fetch` calls. They carry no bootstrap/session contract, CSRF token,
Idempotency-Key, expected revision/ETag, operation receipt, or runtime/config
epoch. Examples include `model-settings.js:395-466`,
`documents.js:698-803`, `trash.js:65-93`, `retrieval-settings.js:55-64`, and
`vector-index-health.js:82-98`.

The server-side legacy review found no application authentication/CSRF boundary.
The UI cannot compensate for that absence: any caller that reaches the local
HTTP listener reaches the same mutation surface, and a network-uncertain retry
cannot distinguish an uncommitted request from an already-applied operation.

### P1: “citation verified” is an unsupported semantic claim

`ask.html:75-85` labels the panel “引用校验”. `ask.js:624-634` maps both
`EXACT` and `PARTIAL` to “引用已校验”. The underlying legacy grounding logic
was shown in the RAG review to accept weak lexical overlap and to return the
original answer even after a refusal decision. This UI wording therefore turns
structural/heuristic metadata into a user-facing claim of semantic support.

The current Go UI deliberately uses the narrower statement that citation
numbers point to material used for the current answer; it does not inherit this
legacy panel, metric vocabulary, or score presentation.

### P1: destructive confirmations overstate completion and recoverability

`trash.js:5-6` promises that permanent delete clears original files, extracted
text, chunks, vectors, and caches. `vector-index-health.js:88-98` reports “已删除”
immediately from a response count. The legacy service review found swallowed
vector failures, incomplete lineage, no backup prerequisite, and no verified
repair/purge receipt. A Java success response is not proof that all named data
was removed, nor that cleanup can safely be retried.

Browser `confirm()` is the only interlock for purge, reindex, and repair. It
does not bind the confirmation to an immutable target/revision or expose a
dry-run plan. A changed resource or double click can therefore apply a decision
to state different from what the user reviewed.

### P1: diagnostics expose internal paths and endpoints as ordinary UI data

`settings.js:35-48` displays the batch staging directory;
`:54-67` displays Python-worker and Ollama base URLs.
`model-settings.js:246-254` renders provider base URLs in technical details.
Other pages dump entire response objects through `JSON.stringify`. There is no
response allowlist, secret/content classification, redaction contract, or
bounded diagnostic download. This expands any XSS or shoulder-surfing incident
and makes content-free support evidence impossible to guarantee.

### P1: the static shell prevents a strict CSP without rewriting

The tree contains an inline script in `agent-tools.html:43-102`, inline event
handlers in `collections.html:22` and `documents.html:63,125`, and widespread
inline `style` attributes. It cannot run under a strict nonce/hash-free Content
Security Policy. There is no checked security-header contract, Trusted Types
policy, subresource manifest, or build-time asset identity.

### P1: there is no real-browser product or accessibility gate

The legacy “UI tests” primarily call `Files.readString` and assert that labels,
URLs, or version words exist. They do not start the application in an
independent browser, execute JavaScript, exercise cookies/CSRF, inject hostile
API values, verify focus/keyboard/Chinese IME behavior, test two tabs or
restart, inspect external requests, or run at 200% zoom/forced colors/reduced
motion. The repository has no Playwright/Selenium/WebDriver runner in this
surface; legacy documentation explicitly treats browser automation as
unnecessary.

Consequently, the presence of ARIA labels and a few focus calls is not evidence
that any of the 47 files forms an accessible or secure user workflow.

## Disposition

All 47 files are `DROP`; there are no HTML, JavaScript, CSS, route-name, DOM,
or design-system exceptions. Their product ideas follow the already-reviewed
feature decisions: the Java Agent/Memory/Evaluation/Batch/Notification/Vector
surfaces stay deferred or dropped, and the core document/collection/Ask ideas
are implemented and hardened in the current Go product rather than ported.

The current Go embedded UI is a separate `KEEP/HARDEN` surface. Its citation
wording boundary is enforced by Go tests, and the missing true browser gate is
specified in [`go-browser-gate.md`](./go-browser-gate.md). That specification
is still honest about status: no release PASS is possible until an actual
Windows `mindweaver.exe` is driven by an independent, pinned, offline browser
against an isolated Vault and literal-loopback fake Ollama.

## Executed checks

The Git-blob verifier must report exactly 47 unique files at the frozen baseline
and exactly 10,223 total lines. `git diff --check` is also a commit gate. No
legacy page was served, no browser session was used, no provider was contacted,
and no user data or Java mutation was executed.
