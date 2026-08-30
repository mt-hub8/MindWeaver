# MindWeaver Ideas CLI MVP

Status: IMPLEMENTED development slice; IDEA-001 is not PASS or
release-qualified. This document defines the bounded MVP and its stop
condition. It is not a claim that formal long-term Memory or Agent capability
is complete.

## Product outcome

The first useful Coding Agent slice is a local CLI that can:

1. capture the root user's visible Codex prompts and each turn's final visible
   assistant answer;
2. import an equivalent bounded session from a file or stdin;
3. extract conservative, evidence-linked ideas, goals, constraints, questions,
   assumptions, rationale, evidence, decisions, learnings, concerns, open items,
   and next actions;
4. keep user items and assistant context visibly separate; and
5. publish one new no-overwrite report directory containing `report.json` and
   `report.md`; and
6. optionally ask one literal-loopback Ollama model to rank only the already
   verified user-item IDs, without generating or rewriting report content.

The MVP is useful without a model. Its rule engine is deliberately
deterministic and conservative: ambiguous text is omitted rather than promoted
to a stronger claim. Optional model output is non-authoritative selection data:
it cannot add candidates, explanations, facts, sources, attribution, or
relations.

## Supported commands

```text
mindweaver ideas hooks print   --scope user|repo
mindweaver ideas hooks install --scope user|repo
mindweaver ideas hooks status  --scope user|repo
mindweaver ideas sessions
mindweaver ideas current extract -output <new-directory> \
  [-ollama-model <model>]
mindweaver ideas extract -session <sha256:id> -output <new-directory> \
  [-ollama-model <model>]
mindweaver ideas extract -input <file|-> \
  -input-format session-json|chat-jsonl|note \
  -output <new-directory> \
  [-ollama-model <model>]
```

`mindweaver ideas hook codex` is an internal callback. It is intentionally not
shown in normal help.

`ideas current extract` does not mean "the globally newest session." It hashes
the command's exact current working directory with the private spool key and
selects only one open captured root session whose latest event has that same
project identity and which already contains visible turns. No match fails as
not found; concurrent open matches fail as an identity conflict and require
`ideas sessions` plus explicit `-session`. It never widens to another project
or silently chooses between parallel Codex tasks.

`-ollama-model` explicitly enables optional local ranking for `extract` and
`current extract`. The default endpoint is `http://127.0.0.1:11434`, and
`-ollama-endpoint` accepts only an `http` literal loopback address while
`-ollama-timeout` is bounded to 60 seconds. Endpoint or timeout flags without a
model are rejected. The CLI never probes, downloads, or automatically chooses
a model, never uses ambient proxy settings, never follows redirects, and never
calls a model from the Codex Hook callback.

`hooks print` previews the complete merged Codex `hooks.json` without writing.
`hooks install` atomically creates a missing file with four separate,
matcher-free command groups. If `hooks.json` already exists and needs a merge
or upgrade, install fails closed with `ideas.hook_identity_conflict`; the user
must review `hooks print` and apply that merge manually. Existing semantically
installed bytes are a no-write success. `hooks status` reports
`absent|partial|drifted|conflict|invalid|installed`. Installation never claims
that Codex has reloaded or trusted the command: activation is always reported
as `unknown`. The user must first review and trust it through `/hooks`, then
start a new Codex task to verify activation.

Repository-scope configuration contains a machine-local executable path and
must not be committed.

## Codex capture contract

The callback accepts only the current official shapes for:

- `SessionStart`;
- root `UserPromptSubmit`;
- `Stop`; and
- `SessionEnd`.

It does not read `transcript_path`. It does not capture hidden reasoning,
Commentary, tool arguments, tool results, environment variables, or arbitrary
transcript records. Subagent `UserPromptSubmit` events are ignored. A repeated
`Stop` with identical safe content is an idempotent replay; the same turn with
changed final content is retained as a separate visible event.

The installed Codex hook has a 3 second process timeout. The callback does not
claim that a Go context can interrupt a blocked operating-system stdin read;
Codex owns that outer process deadline. Success, ignore, and exact replay
produce zero stdout/stderr bytes and exit `0`. Every failure produces zero
stdout bytes, one stable content-free error code on stderr, and exit `1`. It
never returns exit `2`, which Codex may interpret as a blocking hook decision.

## Local capture boundary

On Windows the spool has one fixed production location:

```text
KnownFolder(LocalAppData)\MindWeaver-Ideas-Codex-Spool-v1
```

The path cannot be overridden through an environment variable or callback
argument. The implementation requires a fixed local registered volume,
rejects reparse objects and multiply-linked records, and uses a protected
current-user-only ACL. This is not application-level encryption and does not
claim protection from the same user or an administrator. A cross-process lock
serializes the bounded global scan and publication.

Before the first persistent record write, visible text is normalized and passed through
the frozen redaction policy. Raw session IDs, turn IDs, working directories,
transcript paths, models, permission modes, and dedupe values are not stored.
Private session/project/turn identities use a local random HMAC key. Public
event identity is opaque. Dedupe values remain private filenames and never
enter reports.

Each record is written to a same-directory temporary file, flushed, closed,
and published with Windows no-replace/write-through semantics before the
parent directory is flushed. Capacity is bounded per session and globally.
Unsafe existing state is rejected rather than repaired silently.

## Distillation and evidence contract

Input is fully decoded, normalized, redacted, sliced, classified, related,
verified, and rendered in memory before publication. Any failure returns a
zero bundle.

The strict session input accepts only:

```json
{
  "schema_version": 1,
  "session_id": "sha256:<64 lowercase hex>",
  "turns": [
    {
      "event_id": "sha256:<64 lowercase hex>",
      "ordinal": 1,
      "role": "user",
      "text": "visible text"
    }
  ]
}
```

Unknown, duplicate, or mis-cased fields; invalid UTF-8 or surrogate escapes;
hidden/tool fields; non-contiguous ordinals; and size/depth/token-limit
violations fail closed.

Every extracted item includes:

- a deterministic item ID;
- kind and user/assistant attribution;
- the exact safe statement and rule ID; and
- one or more sources binding the canonical redacted event, role, ordinal,
  half-open UTF-8 byte span, content-bound event ID, and span hash.

The core derives public session/event identities only after redaction. A final
verifier recomputes the input digest, content-bound event IDs, complete item
set, span boundaries and hashes, roles, rule IDs, and the complete ordered
relation set from canonical eligible slices before either renderer runs.

Ordinary candidate relations are labelled `rule_derived_candidate`, while
`responds_to` is only a structural adjacency. `evolves_from` is narrower: it
is emitted only when a user uses a strong first-person change cue that also
explicitly refers to the previous idea, the previous user turn contains
exactly one eligible idea, decision, or assumption, the current turn contains
exactly one classified item, and both endpoint items have one unambiguous
source occurrence. Unanchored “I changed my mind” text is still extracted as
an idea but is not linked to an arbitrary earlier topic. The relation remains a
`rule_derived_candidate`, not proof that the two statements concern the same
topic. The rule never infers a change from an assistant answer, a weak “now I
think” phrase, or reported Agent/document speech. Relations are not asserted
as semantic truth, and the vocabulary contains no `answers` or `contradicts`
claim.

The output bundle is opaque and internally verified before publication.
Accessors return deep copies, and the publisher accepts only a bundle produced
and verified by the core. Both reports are staged and exposed by one
no-overwrite directory rename. Before the rename the publisher enumerates the
staging directory and verifies the exact two-file set, retained identities,
sizes, link counts, and content hashes. After the rename it reopens the final
directory through the retained parent capability, repeats the exact-set and
content verification, and holds the verified file handles through the parent
directory sync. A failure after that rename can leave an uncertain publication
outcome which must be inspected before retrying. On unsupported platforms
publication fails before staging any output.

This is a crash, namespace-replacement, and competing-destination contract for
a single publisher. It is not a tamper-resistance boundary against another
process running as the same Windows user. Such a process can modify any
user-owned report after publication and Windows directory oplocks do not block
child-content mutation. The MVP therefore does not claim same-user adversarial
isolation; that would require a different security principal, a filesystem
driver, or deprecated NTFS transactions.

When optional Ollama ranking is enabled, the complete verified `user_items`
set is sent once through the existing loopback-only client only if the whole
bounded prompt fits; otherwise the report records an input-limit fallback
without a partial model call. The model must return exactly one strict JSON
object containing only `schema_version: 1` and
an ordered, unique subset of at most 12 existing user-item IDs. Unknown,
assistant, repeated, malformed, fenced, or trailing output is rejected. The
model cannot change statements, kinds, sources, roles, relations, or the base
input digest. Timeout, unavailability, invalid response, input limit, and
outcome uncertainty create a content-free `model_assistance` fallback section
and still publish the deterministic evidence report; caller cancellation stops
publication. Prompt, raw response, endpoint, model name, and error text are not
stored in either report. The Markdown section labels every ranking as optional,
local, and non-authoritative.

## Reuse decisions

The implementation reuses or adapts narrow ideas rather than importing a
second Agent runtime:

- Official Codex source (`openai/codex` commit
  `dde85b435b16994f956bce08e5fb796ed94c27fd`) is the schema and `hooks.json`
  authority. Private transcript formats are not treated as an API.
- MetaGPT commit `11cdf466d042aece04fc6cfd13b28e1a70341b1f`
  contributes the useful design ideas that insights retain source evidence,
  messages retain actor provenance, and processing follows an explicit SOP.
  Its Python multi-Agent runtime and lossy memory implementations are not
  imported.
- OpenManus commit `3309bf4e416fb1c74b008f3e86494439a31bad53`
  contributes strict structured contracts and explicit state-machine thinking.
  Its short in-memory history and raw thought/tool logging are rejected.
- The default MVP has no model or network dependency. Its optional Ollama
  ranking reuses the existing fixed literal-loopback client and is limited to
  selecting existing verified IDs; no second Agent runtime or provider registry
  is introduced.
- Codex App Server is not a first-release capture dependency. A future history
  importer may use its public, versioned thread APIs only as an explicit user
  action; it remains LATER, must not scrape private transcript files, and does
  not block this Hook/file/stdin MVP.

## Explicit exclusions

This slice does not implement or revive:

- the abandoned Agent Memory SQLite migration, Store, extraction Job, IPC, or
  FTS work;
- formal long-term memory admission, recall, edit, or forget;
- ACP, `codex-acp`, a Node/Python Codex SDK, or an MCP server;
- transcript scraping, hidden reasoning capture, or raw tool logging;
- vector/embedding/hybrid retrieval, natural-question understanding, RL, or a
  multi-Agent framework;
- model-generated candidates, summaries, reasons, cross-session ranking, or a
  learned admission policy;
- cloud storage, telemetry, browser automation, installer, signing, or a third
  executable.

The shipped executable set remains exactly `mindweaver.exe` and
`mindweaver-pdf.exe`.

## MVP verification and stop condition

The slice is complete only when executable tests prove:

- strict official Hook decoding, root/subagent separation, zero-output callback
  behavior, replay, changed `Stop`, configured process timeout, and panic
  containment;
- owner-only/reparse/hardlink/lock/no-replace/capacity spool behavior;
- Hook config preview/install/status, preview-only preservation of unrelated
  hooks, existing-file conflict, linked worktree resolution, atomic
  create-without-replace, uncertain-outcome handling, and exact reinstall
  no-op;
- Hook capture through fail-closed unique-open-current or explicit session
  selection, deterministic extraction, and no-overwrite single-directory
  publication with uncertain-outcome handling;
- file and stdin imports, bilingual taxonomy and negative corpus, redaction,
  source/hash/rune mutation rejection, relation bounds, viewpoint evolution,
  and byte determinism;
- optional Ollama strict-output ranking, no-model byte compatibility,
  loopback/proxy/redirect denial, cancellation, timeout, invalid response, and
  deterministic fallback without evidence mutation;
- the public command/production package contract and exactly two reproducible
  PE artifacts; and
- focused/full tests, vet, CI, and all clean-tree gates that are actually
  runnable on the final integration state.

Once those conditions are met, this task stops. Model-generated candidates or
reasons, learned ranking/admission, and formal memory are separate future
slices.

Ordinary tests never redirect the production KnownFolder spool through an
environment variable and never write a successful Hook capture into the
current user's profile. Before IDEA-001 can become PASS, a real built-PE
Hook-to-sessions-to-explicit-extract chain must run under an isolated temporary
Windows account with its own loaded profile, followed by verified profile and
account cleanup. A non-administrator environment reports that qualification as
blocked rather than falling back to the current user's spool. This is a
process/profile qualification boundary, not an unimplemented product path.
