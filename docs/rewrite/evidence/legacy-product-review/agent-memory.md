# Agent and Memory source review

## Frozen baseline and scope

- Baseline: `0df22ddaf02c64bf73a7df12cd5fea6b52632c73`
- Machine-readable evidence: [`agent-memory.csv`](./agent-memory.csv)
- Verification: `pwsh -NoProfile -File docs/rewrite/evidence/legacy-product-review/verify.ps1 -Manifest docs/rewrite/evidence/legacy-product-review/agent-memory.csv`
- Every row covers the complete file (`1..line_end`) and records its SHA-256.

The legacy set contains every source and test under the `agent` and `memory`
packages plus the named controller, DTO, enum, entity, repository, Rabbit,
service, configuration, schema, RAG-consumer, and static UI adapters. The
unrelated `InMemoryMultipartFile` and the Evaluation-only test double
`ReversedRankingInMemoryVectorStore` are intentionally owned by other
partitions, not selected by a substring accident.

| Surface | Files | Lines | Decision |
| --- | ---: | ---: | --- |
| Legacy direct production packages | 20 | 1,550 | `DROP` |
| Legacy product adapters | 59 | 4,505 | `DROP` |
| Legacy MySQL schema | 3 | 167 | `DROP` |
| Legacy static UI | 8 | 1,361 | `DROP` |
| Legacy tests | 22 | 2,507 | `DROP` |
| Current Go negative-surface contract/test | 2 | 1,678 | `HARDEN` |
| **Total** | **114** | **11,768** | frozen in CSV |

There are zero Java implementation exceptions. Agent/Profile and Memory remain
absent from the Go production package graph, routes, schema, and UI.

## Findings

### P1: duplicate delivery can execute the same Agent task twice

`AgentTaskExecutor.java:39-58` reads a task, checks its status in memory, writes
`RUNNING`, and immediately performs the workflow inside one transaction. There
is no atomic `PENDING -> RUNNING` claim, lease token, version predicate, or
second-consumer exclusion. Two Rabbit deliveries can both pass the check and
call the model. `AgentTaskConsumer.java:17-20` adds no delivery identity or
claim boundary. The unit tests mock repositories and the workflow; none runs a
two-consumer broker/database race or kill/restart lease takeover.

### P1: database commit and Rabbit publication are not atomic

`AgentTaskService.java:52-92` creates the task, events, and steps and publishes
to Rabbit inside the same database transaction. The message may become visible
before the MySQL commit, and a later rollback cannot retract it. Conversely,
the catch path marks `FAILED` and then throws at lines `93-108`, which makes its
own transaction outcome dependent on rollback rules. There is no transactional
outbox or restart reconciliation proving exactly one durable runnable task.

### P1: network/model work is held inside a database transaction

`AgentTaskExecutor.java:39-58` keeps its transaction open across the whole
workflow; `AgentTaskWorkflowService.java:235` performs a provider call in that
scope. A slow or uncertain call holds database resources, while rollback can
erase the local audit changes without undoing the external effect. There is no
cancel/retry budget or outcome-uncertain user state.

### P1: Memory and profile text is executable prompt material

`MemoryContextAssembler.java:73-93` concatenates stored titles and contents
directly into the prompt. `AgentProfileService.java:99-123` similarly accepts a
user-controlled system instruction. The data has no immutable provenance,
content trust class, instruction delimiter, aggregate prompt-byte budget, or
purge lineage. A stored “memory” can therefore change later model behavior as
an instruction. The 8,000-character per-record limit does not close this
boundary.

### P1: step audit JSON is deliberately made invalid when oversized

`AgentWorkflowJsonCodec.java:20-31` serializes a map and then truncates the JSON
string at an arbitrary character boundary. The resulting value is not JSON;
`deserialize` falls back to a `raw` field at lines `35-44`. This contradicts the
claimed durable, inspectable step receipt and can persist partial source/model
content without a typed truncation record.

### P2: Memory operations scale with total history, not the requested window

`MemoryService.java:113-156`, `:168-171`, and `:208-239` repeatedly load all
rows and filter/sort in memory; conflict detection is quadratic. Selection then
updates usage one record at a time through `MemoryContextAssembler.java:64`.
This is neither bounded context assembly nor an atomic record of what a model
actually consumed.

## Closed-loop assessment

The legacy tests cover fixed tool order, request/response mapping, in-process
JPA behavior, and some scope filters. They do not cover a real Rabbit redelivery,
parallel claim, provider timeout, process kill, restart recovery, cancellation,
browser security, prompt injection, or purge. Consequently, the many entities,
events, steps, and pages are implementation volume rather than a proven Agent
or Memory product loop.

## Current Go boundary

`v2/openapi/v1/contract.go:27-30` reserves Agent, Memory, Evaluation,
Embedding, Rerank, and Vector route segments as forbidden. The executable
negative test at `contract_test.go:176-184` rejects adding
`internal/agent` to the production surface. Keep this absence gate and harden it
as the real package/schema/UI closure evolves. Do not create dormant Agent or
Memory tables, jobs, routes, feature flags, or provider purposes.

Future Agent/Memory work is a new zero-to-one product decision. It must first
prove user workflow, scoped data, bounded prompt construction, injection
handling, deletion lineage, durable claims, cancellation, and browser operation
against the then-current Go core; no Java class, schema, Rabbit choreography,
or profile catalog is a starting implementation.

## Executed checks

The evidence verifier and `git diff --check` are commit gates. No Java service,
Rabbit broker, model, migration, or user data was executed.
