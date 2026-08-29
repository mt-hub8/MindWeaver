# Grounded Answer Contract and Citation Verification (V18.0)

V18.0 makes RAG answers stricter and more auditable. Retrieval correctness is not enough: the final answer must stay inside the final context, cite concrete chunks, and refuse when evidence is missing.

## Goals

- Add `GroundedAnswerContract`.
- Assemble a traceable `GroundedContextBundle`.
- Require citation keys such as `[1]` for key conclusions.
- Verify citations against final context.
- Detect unsupported claims with deterministic heuristics.
- Refuse or mark uncertainty when context is insufficient.
- Show Answer Grounding Diagnostics on the Ask page.
- Feed grounding results into V14 Knowledge Health.

## Why Retrieval Correctness Is Not Enough

A retriever can return useful chunks while the generator still adds unsupported facts, cites weak evidence, or answers no-answer questions too confidently. V18 adds a contract, citation verification, unsupported claim detection, and refusal policy around final answer generation.

## Grounded Answer Contract

Rules:

- only use final context;
- do not invent facts, document names, section names, paths, config keys, API paths, or versions;
- cite every key conclusion;
- refuse when context is insufficient;
- mark inference as inference;
- call out conflicting sources.

## Contract Modes

- `STRICT`: for API docs, contracts, config keys, versions, and exact technical facts.
- `BALANCED`: default mode for normal knowledge-base Q&A.
- `EXPLORATORY`: for learning and brainstorming, with explicit inference labels.

## Context Assembly

`GroundedContextAssembler` converts retrieval chunks into a `GroundedContextBundle`. Each chunk carries chunk id, document id, document title, collection id, version, doc type, section path, chunk type, text, rank, score, retrieval source, direct/expanded flags, and citation key.

Citation keys are assigned in final context order: `[1]`, `[2]`, `[3]`. Duplicate chunks are removed by chunk id or normalized content hash. Budget truncation is recorded.

## Prompt Contract Builder

`GroundedAnswerPromptBuilder` writes answer rules into the prompt: use only material, cite claims, refuse on missing evidence, do not fabricate metadata, and report conflicts. If context is empty, it builds a refusal prompt.

## Citation Model

`Citation` stores citation key, answer span, chunk id, document id, document title, collection id, version, section path, quote snippet, support level, verification status, and warning.

Support levels: `EXACT`, `PARTIAL`, `WEAK`, `UNSUPPORTED`, `UNKNOWN`.

Verification statuses: `PENDING`, `VERIFIED`, `FAILED`, `HEURISTIC`.

## Citation Verification

`CitationVerificationService` is heuristic by default and does not call an LLM judge. It checks citation existence, final-context membership, collection/version scope, exact symbols/config keys/API paths/numbers/versions, and keyword overlap.

## Unsupported Claim Detection

`UnsupportedClaimDetector` flags key claims without citations, invalid citation keys, unsupported symbols, unsupported versions, unsupported numbers, and hallucination risk when there is no context.

## Refusal Policy

`RefusalPolicyService` refuses when context is empty, retrieval confidence is too low, the query is ambiguous, a requested version is not found, strict mode has no evidence, or all citations are unsupported.

## Answer Grounding Score

```text
groundingScore =
  CitationCoverage * 25
+ CitationAccuracy * 30
+ (1 - UnsupportedClaimRate) * 25
+ ContextUsage * 10
+ RefusalCorrectness * 10
```

Levels: 可信, 基本可信, 需要复核, 不可信.

The score is heuristic. It measures grounding quality, not absolute truth.

## Ask Page

The Ask page shows 可信回答状态, 引用校验, 未被资料支持的主张, refusal reason, enriched citation source cards, and raw grounding JSON in technical details.

## V14 Knowledge Health

V14 case results can store context bundle JSON, citation verification JSON, unsupported claim report JSON, refusal decision JSON, and grounding score JSON. Generation metrics include grounded citation accuracy, grounded faithfulness, unsupported claim rate, refusal decision, and answer grounding score.

## V17 Query Understanding Integration

V17 extracted versions, code symbols, config keys, and API paths are passed into citation verification. If a specific version or symbol is required but absent from the cited chunk, the citation becomes weak or unsupported.

## Boundaries

V18 does not add a cloud judge, complex LLM-as-a-judge platform, multi-agent planner, external search, new search engine, packaging work, or large UI redesign.

## FAQ

**Does V18 call a real LLM judge?**  
No. The first version uses prompt contracts and heuristic verification only.

**Can a heuristic verifier prove truth?**  
No. It checks grounding signals and leaves uncertain cases as weak or unknown.

**Why refuse instead of answering partially?**  
When evidence is missing, refusal is safer than a confident unsupported answer. Partial answers should explicitly state what can and cannot be confirmed.
