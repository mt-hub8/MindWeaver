# Go rewrite hard-deadline checkpoint — 2026-08-27 06:21 +08:00

This checkpoint records the mandatory five-hour stop. It is a resumable state,
not a completion or release-readiness claim. No Java/MySQL migration work is to
be revived.

## Controller

- Branch: `codex/go-rewrite`
- Last completed controller commit:
  `5efd3c0b7bdcb39418504296f621757c91be7fbe`
- Worktree at checkpoint creation: clean before adding this checkpoint.
- The 20-run Windows runtime interruption evidence was integrated in `5efd3c0`.
  RUN-002 and RUN-003 deliberately remain `IMPLEMENTED`, not `PASS`.
- Runtime evidence received two independent deletion reviews. Its committed
  contract now binds exact commit/binary/fixture identities, the complete four
  state count maps, exact identity fields, cross-state identity continuity,
  distinct ingestion blobs, unique run identities and canonical JSON.
- The controller stopped the unfinished read-only BLOB-001 and REL-001 audits;
  neither produced an acceptance decision and neither ledger row was changed.

## Paused candidate: ARC-003 ownership

- Branch: `codex/arc003-capability-ownership`
- Clean checkpoint: `fd377faaa0312e82fddb88140a05e59bf31b5868`
- It freezes 17 CORE implementation identities, 13 LATER absence rows, and
  derives route/table/command/package/test closure from the real product
  surface rather than trusting the matrix alone.
- Focused OpenAPI/production tests and vet passed.
- It is not accepted yet: full module tests, full vet, CI, standalone, and an
  independent P0/P1 review were not completed before the deadline.
- Resume with the full module test from that worktree, then review before any
  cherry-pick.

## Paused candidate: RET product pruning

- Branch: `codex/ret003-final-8aadf60`
- Clean checkpoint: `a5c00dc5d0471b97eafd3c59ac4aaae631b84bcb`
- Commits: `66f0450` then `a5c00dc`.
- Proposed product decision: CORE supports one unchanged continuous source
  phrase through bounded FTS5 trigram search. Natural-question understanding,
  token OR, rewriting, fusion and semantic retrieval move to LATER and remain
  unimplemented; every measured prior candidate remains rejected.
- Focused retrieval, RAG, WebUI, and single-run OpenAPI checks passed.
- It is not accepted yet: full module tests/vet/CI/standalone/inventory checks
  and final independent review were not completed before the deadline.

## Paused candidate: CUT-002 offline extraction

- Branch: `codex/cut002-offline-vendor-8aadf60`
- Clean checkpoint: `583536da3beddfdb7fbb0d39fc5f7734ea4cc25e`
- Commits: `c2871b1` canonical vendor, then `583536d` gate rewrite.
- The monorepo tracked-only, empty-cache, network-off standalone run passed.
  The rewrite also addresses LF attributes, exact Go 1.27 CI, vendor-mode
  buildinfo, first-party exclusion, and both repository layouts.
- It is not accepted yet: the extracted repo-root standalone run was stopped
  at the deadline after partial success; README/evidence synchronization and
  final independent review remain.
- A retained temporary qualification checkout was reported at
  `C:\Users\24281\AppData\Local\Temp\mindweaver-cut002-root-layout-f1642bd416054333a3a46fce8d6c1802`.
  It is test residue, not product state; do not delete it with a broad command.

## Resume order

1. Reconfirm this controller worktree is clean and read this checkpoint.
2. Finish and review CUT-002 from its exact extracted-repository command; do
   not claim PASS from the completed monorepo run alone.
3. Run and independently review ARC-003 before selective integration.
4. Run and independently review the RET pruning decision; verify that it only
   narrows claims and does not silently expand retrieval behavior.
5. Integrate accepted candidates one at a time, then regenerate all production
   source and PE identities after the final vendor/build-mode change.
6. Run full tests, vet, CI, and both standalone layouts. Keep browser,
   installer/signing, real clean-machine, and natural-question capabilities
   BLOCKED/NOT_IMPLEMENTED until their external evidence exists.
