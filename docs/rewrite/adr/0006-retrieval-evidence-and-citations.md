# ADR 0006: Retrieval evidence and citations

- Status: Accepted
- Date: 2026-08-23

## Decision

Search, Ask, Agent, and Evaluation use one retrieval engine and one immutable
RetrievalSnapshot contract. Collection membership is many-to-many; an explicit
empty scope remains empty. Candidate scores preserve stage and scale provenance
instead of pretending BM25, vector, fusion, and rerank scores are comparable.

Before committing a snapshot, selected candidates are revalidated against
document lifecycle, active generation, generation readiness, collection scope,
and purge state. An AnswerContext then records the exact, ordered, possibly
truncated text actually sent to the model. A citation may reference only a
context item belonging to that same answer.

## Consequences and verification

- No stale, failed, trashed, purging, foreign-generation, or foreign-scope chunk
  is retrievable.
- Strict mode publishes only a verified answer or a system-controlled refusal.
- Model-visible prompt segments and memory consume one explicit token budget and
  participate in lineage, retention, backup, and purge.
- Scope-leak, active-generation isolation, exact-context citation, refusal, and
  deterministic golden-corpus tests gate release.
