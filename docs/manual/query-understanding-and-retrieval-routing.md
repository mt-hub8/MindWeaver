# Query Understanding and Retrieval Routing (V17.0)

V17.0 adds a rule-based query understanding layer before RAG retrieval. It lets MindWeaver infer query intent, metadata constraints, and retrieval strategy automatically, without asking users to understand every retrieval parameter.

## Goals

- Identify query type, including version-specific, latest-version, code symbol, config key, API path, summary, comparison, ambiguous, and no-answer-risk queries.
- Extract collection, version, doc type, source, status, tags, code symbols, config keys, and API paths.
- Route retrieval to vector, hybrid RRF, rerank, and context expansion.
- Avoid blind global search for ambiguous questions.
- Emit diagnostics for Ask page display and V14 Knowledge Health evaluation.

## Query Type Design

`QueryType` includes `SINGLE_DOC_FACT`, `MULTI_DOC_COMPARE`, `VERSION_SPECIFIC`, `LATEST_VERSION`, `CODE_SYMBOL`, `CONFIG_KEY`, `API_PATH`, `METADATA_FILTER`, `SUMMARY`, `TROUBLESHOOTING`, `NO_ANSWER_RISK`, `AMBIGUOUS`, and `GENERAL_CHAT`.

The first implementation is heuristic and deterministic. It does not call a real LLM by default.

## Metadata Hint Extraction

`QueryMetadataExtractor` extracts version hints, doc type hints, status hints, code symbols, config keys, and API paths. User-selected filters have higher priority than inferred hints. Automatic inference must not overwrite explicit user choices.

## Lightweight Query Rewrite

`QueryRewriteService` generates `normalizedQuery`, `keywordQuery`, `semanticQuery`, `symbolQuery`, and `versionAwareQuery`. Rewrite is rule-based and preserves versions, symbols, config keys, and API paths.

## Retrieval Routing Policy

`RetrievalRoutingPolicyService` maps query understanding to a `RetrievalRoutingDecision`.

- Code symbols, config keys, and API paths use `HYBRID_RRF_RERANK` with adjacent context.
- Version-specific queries add a version filter.
- Latest-version queries default to `status=ACTIVE` and exclude deprecated chunks.
- Summary queries use higher recall and `PARENT_AND_ADJACENT` context.
- Multi-document comparison increases candidate depth.
- Ambiguous queries require clarification instead of blind global search.
- No-answer-risk queries increase refusal tendency in the final prompt.

## Clarification Guard

`QueryClarificationGuard` blocks risky global searches when a query is too short, has no entity, has low confidence, or the corpus/collection count is above configured thresholds.

```properties
app.query-understanding.clarification-enabled=true
app.query-understanding.min-confidence=0.55
app.query-understanding.max-global-search-documents=10000
```

## Ask Page Diagnostics

The Ask page shows query type, confidence, extracted hints, routing strategy, filters, topK settings, rerank, context expansion, rewritten queries, warnings, and clarification messages.

## V14 Knowledge Health Integration

Evaluation runs can carry `enableQueryUnderstanding=true`. Baseline runs use the original query and fixed strategy; candidate runs use query understanding and retrieval routing.

V17 adds `QueryTypeAccuracy`, `FilterExtractionAccuracy`, `VersionExtractionAccuracy`, `CollectionRoutingAccuracy`, `ClarificationPrecision`, and `ClarificationRecall`. If expected fields are missing, the metric is `UNKNOWN`.

## Relationship to V15 and V16

V17 does not replace V15 Hybrid Retrieval. It selects when to use vector, hybrid, RRF, rerank, and context expansion.

V17 does not replace V16 Vector Index Isolation. It uses collection/version/status filters and diagnostics to reduce routing mistakes and make pollution easier to detect.

## Boundaries

V17 does not implement an LLM planner by default, agents, multi-round planning, external search, new search engines, or UI rule editors.

## FAQ

**Q: Why not always search all documents?**  
A: Ambiguous global retrieval can mix unrelated projects and versions. Clarification is safer.

**Q: Does V17 call cloud LLMs?**  
A: No. Query understanding and rewrite are rule-based by default.

**Q: Can users still select a collection manually?**  
A: Yes. Manual selection has higher priority than inferred hints.
