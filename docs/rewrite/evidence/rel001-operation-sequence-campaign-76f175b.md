# REL-001 deterministic knowledge-lifecycle campaign

Status: **NOT_IMPLEMENTED**

- Code commit: `76f175b32ebe4a56e65ba2db64aa92728fb07f1d`
- Parent: `6a7995ff462d7a1891af4638513660316be27ab2`
- Recorded: 2026-08-28 (Asia/Shanghai)
- Production changes: none
- Corpus-frame size: 1,406 bytes
- Corpus-frame SHA-256:
  `b969781b5b81ac1a4bb8093dcbbe6b0c3239bbcbcb9645d0d5947666cf84e204`

This addendum closes the repository-owned deterministic-corpus facet that was
left open by `rel001-operation-sequence-f6d4bd8.md`. It does not promote the
conjunctive `REL-001` row.

## Frozen input corpus

`internal/lifecycle/TestKnowledgeLifecycleOperationSequenceCampaignV1` uses the
same real SQLite, Blob, workbench, lifecycle, collection, ingestion and FTS
stack and the same independent shadow-model oracles as the owner-local fuzz
target.

The pair track is a lexicographic `B(32,2)` de Bruijn cycle over the 32 encoded
five-bit action-slot words: 16 actions multiplied by two modeled document
slots. Appending the first word produces one continuous 1,025-step trace. Its
1,024 adjacent ordered pairs contain every encoded-word pair exactly once,
including the 85 pairs that cross serialized batch boundaries.

For bounded framing, that one continuous trace is divided into 86 batches of
at most 12 words. The batches execute against one retained Vault and one
continuous shadow state; they are not 86 independent scenarios. Six additional
12-step tracks each use a fresh Vault:

- `semantic-shared-purge`;
- `semantic-stale-retry`;
- `semantic-queue-order`;
- `semantic-replay-terminal`;
- `semantic-idempotent-lifecycle`;
- `semantic-active-purge`.

The resulting campaign uses seven Vaults, 92 framed plans and 1,097 submitted
words: one 1,025-step shared-state trace plus six independent 12-step scenarios.

The canonical corpus frame is:

1. the NUL-terminated UTF-8 magic
   `mindweaver.rel001.knowledge-sequence-campaign/v1`;
2. a big-endian `uint16` track count;
3. for each ordered track, a `uint8` name length, name bytes and big-endian
   `uint16` plan count;
4. for each ordered plan, a `uint8` word count followed by the raw words.

The executable contract independently checks the exact counts, bounds, cyclic
endpoint, every pair count, frame size and frame SHA-256. The SHA identifies
the input corpus and its batch boundaries. The 29 outcome names are separately
locked by the committed test source; they are not misrepresented as part of
that corpus digest.

## Frozen minimum branch outcomes

The six fresh-Vault tracks jointly require exactly 29 reviewed minimum
response/model branch classes:

- upload 3, run-one 2, cancel 2 and retry 4;
- trash 3, restore 3 and purge 4;
- collection membership 5, reopen 1 and candidate sweep 2.

This includes exact replay, idempotency conflict, queue ordering, queued and
terminal retry behavior, stale revision conflicts, changed and unchanged
lifecycle transitions, active/stale/shared/last-reference purge outcomes,
membership add/remove/idempotency, adding a trashed document, reopen, retained
candidate sweep and empty sweep.

The shadow state selects the expected class before the production call. A class
is recorded only after its response-specific assertions pass; after `apply`
returns, the per-step projection oracles must also pass for the test to succeed.
Every submitted step checks document, job, membership, Blob candidate, global
search and collection-search projections. Purge, reopen and sweep steps, and
every track endpoint, additionally check SQLite integrity, read-only exact table
counts, the exact Blob filesystem tree and staging state.

This is deliberately a minimum branch set. Some current/stale contexts are
grouped, three actions ignore the encoded slot, and actions against a missing
document can be model no-ops. Therefore the campaign does not claim 32 distinct
production operations, every semantic transition pair, every stale-precedence
combination or every longer sequence.

## Executed gates

On the committed code with Go 1.27.0 for Windows/amd64:

- the complete campaign passed with `-count=10` in 48.377 seconds;
- the refactored lifecycle fuzz target and campaign passed together with
  `-count=3` in 15.835 seconds;
- the owner-local lifecycle package test and vet passed;
- the exact six-target production fuzz closure passed in 41.42 seconds; its
  lifecycle target passed in 5.45 seconds;
- two independent reviews recomputed all 1,024 pairs, 92 plan frames,
  1,097 words, the 1,406-byte frame and its SHA-256, and reported P0=0/P1=0.

## Deliberate non-claims

The campaign is deterministic, single-process and intentionally short. It does
not provide an approved CGO/race lane, physical-media or power-loss faults, a
24–48 hour soak, at least 1,000 randomized process kills, a 100k representative
capacity/SLO result, concurrent scheduler interleavings, or an accepted
long-duration multi-component campaign. It introduces no production hook,
journal, schema or second state machine.

Those release-scale facets remain open, so `REL-001` stays
`NOT_IMPLEMENTED`.

## Reproduction

From `v2/` with the repository-selected Go 1.27.0 toolchain and frozen offline
vendor environment:

```powershell
go test ./internal/lifecycle -run '^TestKnowledgeLifecycleOperationSequenceCampaignV1$' -count=10
go test ./internal/lifecycle -run '^(TestKnowledgeLifecycleOperationSequenceCampaignV1|FuzzKnowledgeLifecycleOperationSequence)$' -count=3
go test ./internal/lifecycle -count=1
go vet ./internal/lifecycle
go test ./qualification/reliability -run '^TestCoreFuzzTargetClosureAndExecution$' -count=1 -v
```
