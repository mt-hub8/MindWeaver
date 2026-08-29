# RAG, conversation and API acceptance on current main `43d2448`

This evidence is bound to committed source
`43d2448c37b5df1d1a2c78a0eec3ee36f7f5a032`. It closes backend and embedded
client contracts only. It does not claim real-browser qualification: browser
presentation, two-tab behavior and accessibility remain exclusively under
`SEC-001`, `UI-001` and `UI-002`.

## Executable witnesses

- `RAG-003`: `internal/rag/service.go` refuses no-context answers without a
  provider call and refuses missing, foreign or malformed citations. The public
  HTTP path persists and returns the same durable refusal outcomes. The embedded
  client displays limitations without claiming semantic citation verification.
- `CON-001`: `internal/store/sqlite/rag_answers.go` binds immutable messages,
  answer sources and provider configuration to a conversation revision. Exact
  replay, conflicting reuse, stale revision and close/reopen behavior are tested.
- `API-002`: `internal/store/sqlite/documents.go` and `rag_answers.go` bind
  idempotency to the request payload before durable work. The embedded client
  freezes upload, collection, conversation and Ask attempts for exact replay.

The focused tests were run ten times from `v2/` with Go 1.27.0:

```text
go test ./internal/rag -run 'TestAsk(PersistsReopensAndIdempotentReplayDoesNotRegenerate|CollectionScopeAndExplicitEmptyScopeNeverWidens|UsesCompleteInputAsOneLiteralPhraseAndDoesNotExpandNaturalQuestions|RejectsMissingForeignAndMalformedCitations)$' -count=10
go test ./internal/store/sqlite -run 'Test(CreateDocumentUploadIsAtomicAndIdempotent|RAGAnswerPersistsExactSourcesConfigVersionAndIdempotency|CitationConstraintPendingReconciliationAndConversationDelete|ConversationAndMessageCursorPaginationSurvivesRestart|ConcurrentCollectionMutationsSerializeAtExpectedRevision)$' -count=10
go test ./internal/app -run 'TestRAGProductHTTPDurableOutcomesAndRestartReconciliation$' -count=10
go test ./internal/webui -run 'TestEmbeddedClient(BindsImmutableMutationAttemptsAndFreezesAskReplay|DoesNotClaimSemanticCitationVerification)$' -count=10
```

All four commands passed. These tests execute the real SQLite store, RAG
service, public HTTP product path and embedded JavaScript contract; they do not
use a fake browser as qualification evidence.
