package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRAGAnswerPersistsExactSourcesConfigVersionAndIdempotency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rag.db")
	clock := newFakeClock(testTime)
	database := openTestStore(t, path, clock)
	config, err := database.SaveOllamaConfig(t.Context(), SaveOllamaConfigParams{
		ExpectedVersion: 0, Endpoint: "http://127.0.0.1:11434",
		Model: "qwen3:8b", Timeout: 30 * time.Second,
	})
	if err != nil || config.Version != 1 {
		t.Fatalf("save config = %#v, %v", config, err)
	}
	if _, err := database.SaveOllamaConfig(t.Context(), SaveOllamaConfigParams{
		ExpectedVersion: 0, Endpoint: "http://127.0.0.1:11434",
		Model: "other", Timeout: time.Second,
	}); !errors.Is(err, ErrProviderConfigRevision) {
		t.Fatalf("stale config update error = %v", err)
	}
	seedRAGDocuments(t, database)
	if _, err := database.CreateConversation(t.Context(), "conversation-1", "本地问答"); err != nil {
		t.Fatal(err)
	}
	tooLong := testBeginAsk("conversation-1", "ask-too-long", "user-too-long", "answer-too-long", nil)
	tooLong.Question = strings.Repeat("界", 342)
	if _, err := database.BeginAsk(t.Context(), tooLong); err == nil || !strings.Contains(err.Error(), "1 to 1024") {
		t.Fatalf("over-search-limit Ask error = %v", err)
	}
	params := testBeginAsk("conversation-1", "ask-key-1", "user-1", "answer-1", nil)
	started, err := database.BeginAsk(t.Context(), params)
	if err != nil || !started.Created || started.ProviderConfig.Version != 1 {
		t.Fatalf("begin Ask = %#v, %v", started, err)
	}

	replayParams := params
	replayParams.UserMessageID = "unused-user"
	replayParams.AnswerMessageID = "unused-answer"
	replayed, err := database.BeginAsk(t.Context(), replayParams)
	if err != nil || replayed.Created || replayed.AnswerMessageID != started.AnswerMessageID {
		t.Fatalf("replay Ask = %#v, %v", replayed, err)
	}
	conflict := replayParams
	conflict.RequestHash = strings.Repeat("b", 64)
	if _, err := database.BeginAsk(t.Context(), conflict); !errors.Is(err, ErrAskIdempotency) {
		t.Fatalf("Ask idempotency conflict = %v", err)
	}
	stale := testBeginAsk("conversation-1", "ask-key-2", "user-2", "answer-2", nil)
	if _, err := database.BeginAsk(t.Context(), stale); !errors.Is(err, ErrConversationBusy) {
		t.Fatalf("parallel Ask conflict = %v", err)
	}

	hits, err := database.Search(t.Context(), "知识库", 8)
	if err != nil || len(hits) == 0 {
		t.Fatalf("search = %#v, %v", hits, err)
	}
	sources, err := database.BindAnswerSources(t.Context(), started.AnswerMessageID, hits[:1])
	if err != nil || len(sources) != 1 || sources[0].Content != hits[0].Content {
		t.Fatalf("bind sources = %#v, %v", sources, err)
	}
	if err := database.CompleteAnswer(t.Context(), started.AnswerMessageID, "资料确认了这一点 [1]。", []int{1}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.BeginAsk(t.Context(), stale); !errors.Is(err, ErrConversationRevision) {
		t.Fatalf("terminal stale conversation revision conflict = %v", err)
	}
	if _, err := database.SaveOllamaConfig(t.Context(), SaveOllamaConfigParams{
		ExpectedVersion: 1, Endpoint: "http://127.0.0.1:11434",
		Model: "qwen3:14b", Timeout: time.Minute,
	}); err != nil {
		t.Fatal(err)
	}
	answer, err := database.GetAnswer(t.Context(), started.AnswerMessageID)
	assertCompletedAnswer(t, answer, err, hits[0].ChunkID, 1)

	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(t.Context(), path, Options{BusyTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	reopened.now = clock.Now
	t.Cleanup(func() { _ = reopened.Close() })
	answer, err = reopened.GetAnswer(t.Context(), started.AnswerMessageID)
	assertCompletedAnswer(t, answer, err, hits[0].ChunkID, 1)
	if answer.Sources[0].Content != hits[0].Content {
		t.Fatalf("reopened source content = %q", answer.Sources[0].Content)
	}
}

func TestBindAnswerSourcesRejectsMissingForeignAndChangedChunks(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, *Store, []ChunkHit) []ChunkHit
	}{
		{
			name: "missing",
			mutate: func(_ *testing.T, _ *Store, hits []ChunkHit) []ChunkHit {
				hits[0].ChunkID = "missing-chunk"
				return hits
			},
		},
		{
			name: "foreign collection",
			mutate: func(t *testing.T, database *Store, _ []ChunkHit) []ChunkHit {
				hits, err := database.SearchCollection(t.Context(), "collection-b", "知识库", 8)
				if err != nil || len(hits) != 1 {
					t.Fatalf("foreign search = %#v, %v", hits, err)
				}
				return hits
			},
		},
		{
			name: "content changed",
			mutate: func(t *testing.T, database *Store, hits []ChunkHit) []ChunkHit {
				if _, err := database.db.ExecContext(t.Context(), `
					UPDATE chunks SET content = content || ' changed' WHERE id = ?
				`, hits[0].ChunkID); err != nil {
					t.Fatal(err)
				}
				return hits
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			database := newRAGStore(t)
			if _, err := database.CreateConversation(t.Context(), "conversation", "Question"); err != nil {
				t.Fatal(err)
			}
			scope := "collection-a"
			started, err := database.BeginAsk(t.Context(), testBeginAsk(
				"conversation", "key", "user", "answer", &scope,
			))
			if err != nil {
				t.Fatal(err)
			}
			hits, err := database.SearchCollection(t.Context(), scope, "知识库", 8)
			if err != nil || len(hits) != 1 {
				t.Fatalf("scoped search = %#v, %v", hits, err)
			}
			hits = test.mutate(t, database, hits)
			if _, err := database.BindAnswerSources(t.Context(), started.AnswerMessageID, hits); !errors.Is(err, ErrAnswerSourceChanged) {
				t.Fatalf("BindAnswerSources error = %v", err)
			}
			answer, err := database.GetAnswer(t.Context(), started.AnswerMessageID)
			if err != nil || answer.Status != MessagePending || len(answer.Sources) != 0 {
				t.Fatalf("answer after rejected bind = %#v, %v", answer, err)
			}
		})
	}
}

func TestCompleteAnswerRevalidatesLifecycleAndScope(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, *Store)
	}{
		{
			name: "document trashed",
			mutate: func(t *testing.T, database *Store) {
				if _, err := database.db.ExecContext(t.Context(), `
					UPDATE documents SET status = 'trashed', updated_at = updated_at + 1
					WHERE id = 'document-a'
				`); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "membership removed",
			mutate: func(t *testing.T, database *Store) {
				if _, err := database.db.ExecContext(t.Context(), `
					DELETE FROM collection_documents
					WHERE collection_id = 'collection-a' AND document_id = 'document-a'
				`); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			database := newRAGStore(t)
			if _, err := database.CreateConversation(t.Context(), "conversation", "Question"); err != nil {
				t.Fatal(err)
			}
			scope := "collection-a"
			started, err := database.BeginAsk(t.Context(), testBeginAsk(
				"conversation", "key", "user", "answer", &scope,
			))
			if err != nil {
				t.Fatal(err)
			}
			hits, err := database.SearchCollection(t.Context(), scope, "知识库", 8)
			if err != nil || len(hits) != 1 {
				t.Fatalf("search = %#v, %v", hits, err)
			}
			if _, err := database.BindAnswerSources(t.Context(), started.AnswerMessageID, hits); err != nil {
				t.Fatal(err)
			}
			test.mutate(t, database)
			if err := database.CompleteAnswer(t.Context(), started.AnswerMessageID,
				"不应发布 [1]", []int{1}); !errors.Is(err, ErrAnswerSourceChanged) {
				t.Fatalf("CompleteAnswer error = %v", err)
			}
			answer, err := database.GetAnswer(t.Context(), started.AnswerMessageID)
			if err != nil || answer.Status != MessageFailed || answer.LimitationCode != "SOURCE_CHANGED" ||
				answer.ErrorCode != "SOURCE_CHANGED" || len(answer.Citations) != 0 {
				t.Fatalf("source-changed answer = %#v, %v", answer, err)
			}
		})
	}
}

func TestCitationConstraintPendingReconciliationAndConversationDelete(t *testing.T) {
	database := newRAGStore(t)
	if _, err := database.CreateConversation(t.Context(), "conversation", "Question"); err != nil {
		t.Fatal(err)
	}
	started, err := database.BeginAsk(t.Context(), testBeginAsk(
		"conversation", "key", "user", "answer", nil,
	))
	if err != nil {
		t.Fatal(err)
	}
	hits, err := database.Search(t.Context(), "知识库", 8)
	if err != nil || len(hits) == 0 {
		t.Fatalf("search = %#v, %v", hits, err)
	}
	if _, err := database.BindAnswerSources(t.Context(), started.AnswerMessageID, hits[:1]); err != nil {
		t.Fatal(err)
	}
	if err := database.CompleteAnswer(t.Context(), started.AnswerMessageID,
		"foreign [2]", []int{2}); !errors.Is(err, ErrInvalidCitation) {
		t.Fatalf("foreign citation error = %v", err)
	}
	answer, err := database.GetAnswer(t.Context(), started.AnswerMessageID)
	if err != nil || answer.Status != MessagePending || len(answer.Citations) != 0 {
		t.Fatalf("answer after foreign citation = %#v, %v", answer, err)
	}
	conversation, err := database.GetConversation(t.Context(), "conversation")
	if err != nil || !conversation.PendingAnswer || conversation.PendingAnswerID == nil ||
		*conversation.PendingAnswerID != started.AnswerMessageID || conversation.Revision != 1 {
		t.Fatalf("pending conversation projection = %#v, %v", conversation, err)
	}
	page, err := database.ListConversations(t.Context(), nil, 10)
	if err != nil || len(page.Items) != 1 || page.Items[0].PendingAnswerID == nil ||
		*page.Items[0].PendingAnswerID != started.AnswerMessageID {
		t.Fatalf("pending conversation catalog projection = %#v, %v", page, err)
	}
	if deleted, err := database.DeleteConversation(t.Context(), "conversation", 1); !errors.Is(err, ErrConversationBusy) || deleted {
		t.Fatalf("pending conversation delete = %v, %v", deleted, err)
	}
	historicalCreated := answer.CreatedAt.Add(time.Microsecond).UnixMicro()
	if _, err := database.db.ExecContext(t.Context(), `
		INSERT INTO conversation_messages(
			id, conversation_id, ordinal, role, status, content,
			provider_config_version, limitation_code, error_code,
			created_at, completed_at, reconcile_after
		) VALUES ('historical-pending', 'conversation', 3, 'assistant', 'pending', '',
			1, NULL, NULL, ?, NULL, ?)
	`, historicalCreated, historicalCreated+time.Minute.Microseconds()); err != nil {
		t.Fatalf("seed historical second pending answer: %v", err)
	}
	reconciled, err := database.ReconcileAllPendingAnswers(t.Context())
	if err != nil || reconciled != 2 {
		t.Fatalf("reconcile = %d, %v", reconciled, err)
	}
	answer, err = database.GetAnswer(t.Context(), started.AnswerMessageID)
	if err != nil || answer.Status != MessageFailed || answer.ErrorCode != "OUTCOME_UNCERTAIN" {
		t.Fatalf("reconciled answer = %#v, %v", answer, err)
	}
	conversation, err = database.GetConversation(t.Context(), "conversation")
	if err != nil || conversation.PendingAnswer || conversation.PendingAnswerID != nil {
		t.Fatalf("terminal conversation projection = %#v, %v", conversation, err)
	}
	if _, err := database.db.ExecContext(t.Context(), "DELETE FROM documents WHERE id = 'document-a'"); err == nil {
		t.Fatal("document deletion unexpectedly bypassed answer provenance RESTRICT")
	}
	if deleted, err := database.DeleteConversation(t.Context(), "conversation", 0); !errors.Is(err, ErrConversationRevision) || deleted {
		t.Fatalf("stale conversation delete = %v, %v", deleted, err)
	}
	if deleted, err := database.DeleteConversation(t.Context(), "conversation", 1); err != nil || !deleted {
		t.Fatalf("conversation delete = %v, %v", deleted, err)
	}
	if deleted, err := database.DeleteConversation(t.Context(), "conversation", 1); err != nil || deleted {
		t.Fatalf("idempotent conversation delete = %v, %v", deleted, err)
	}
	if _, err := database.db.ExecContext(t.Context(), "DELETE FROM documents WHERE id = 'document-a'"); err != nil {
		t.Fatalf("document delete after explicit conversation delete: %v", err)
	}
}

func TestPendingAnswerDeadlineAndBoundedRuntimeReconciliation(t *testing.T) {
	database, clock := newRAGStoreWithClock(t)
	answerIDs := make([]string, 0, 3)
	for index := range 3 {
		conversationID := fmt.Sprintf("deadline-conversation-%d", index)
		if _, err := database.CreateConversation(t.Context(), conversationID, "Deadline"); err != nil {
			t.Fatal(err)
		}
		params := testBeginAsk(conversationID, fmt.Sprintf("deadline-key-%d", index),
			fmt.Sprintf("deadline-user-%d", index), fmt.Sprintf("deadline-answer-%d", index), nil)
		params.RequestHash = strings.Repeat(string(rune('a'+index)), 64)
		started, err := database.BeginAsk(t.Context(), params)
		if err != nil {
			t.Fatal(err)
		}
		answer, err := database.GetAnswer(t.Context(), started.AnswerMessageID)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := answer.ReconcileAfter.Sub(answer.CreatedAt), time.Second+answerReconcileGrace; got != want {
			t.Fatalf("reconcile deadline delta = %v, want %v", got, want)
		}
		answerIDs = append(answerIDs, answer.ID)
	}
	if count, err := database.ReconcileExpiredPendingAnswers(t.Context(), 2); err != nil || count != 0 {
		t.Fatalf("early reconciliation = %d, %v", count, err)
	}
	for _, answerID := range answerIDs {
		answer, err := database.GetAnswer(t.Context(), answerID)
		if err != nil || answer.Status != MessagePending {
			t.Fatalf("unexpired answer = %#v, %v", answer, err)
		}
	}
	clock.Advance(20 * time.Second)
	armedDeadline, err := database.ArmAnswerInvocation(t.Context(), answerIDs[0])
	if err != nil || !armedDeadline.After(time.UnixMicro(clock.Now().UnixMicro()).UTC()) {
		t.Fatalf("arm invocation = %v, %v", armedDeadline, err)
	}
	clock.Advance(12 * time.Second)
	if count, err := database.ReconcileExpiredPendingAnswers(t.Context(), 2); err != nil || count != 2 {
		t.Fatalf("bounded reconciliation first page = %d, %v", count, err)
	}
	if count, err := database.ReconcileExpiredPendingAnswers(t.Context(), 2); err != nil || count != 0 {
		t.Fatalf("armed invocation was reconciled early = %d, %v", count, err)
	}
	armed, err := database.GetAnswer(t.Context(), answerIDs[0])
	if err != nil || armed.Status != MessagePending {
		t.Fatalf("armed answer before deadline = %#v, %v", armed, err)
	}
	clock.Advance(20 * time.Second)
	if count, err := database.ReconcileExpiredPendingAnswers(t.Context(), 2); err != nil || count != 1 {
		t.Fatalf("armed invocation after deadline = %d, %v", count, err)
	}
	if _, err := database.ReconcileExpiredPendingAnswers(t.Context(), maxPendingReconcileBatch+1); err == nil {
		t.Fatal("oversized reconciliation page unexpectedly accepted")
	}
	for _, answerID := range answerIDs {
		answer, err := database.GetAnswer(t.Context(), answerID)
		if err != nil || answer.Status != MessageFailed || answer.ErrorCode != "OUTCOME_UNCERTAIN" {
			t.Fatalf("expired answer = %#v, %v", answer, err)
		}
	}
}

func TestConversationAndMessageCursorPaginationSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	clock := newFakeClock(testTime)
	database := openTestStore(t, path, clock)
	if _, err := database.SaveOllamaConfig(t.Context(), SaveOllamaConfigParams{
		ExpectedVersion: 0, Endpoint: "http://127.0.0.1:11434",
		Model: "qwen3:8b", Timeout: time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	seedRAGDocuments(t, database)
	for _, id := range []string{"conversation-a", "conversation-b", "conversation-c"} {
		if _, err := database.CreateConversation(t.Context(), id, "Conversation "+id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.CreateConversation(t.Context(), "conversation-history", "History"); err != nil {
		t.Fatal(err)
	}
	hits, err := database.Search(t.Context(), "知识库", 8)
	if err != nil || len(hits) == 0 {
		t.Fatalf("search = %#v, %v", hits, err)
	}
	largeAnswer := strings.Repeat("答", 80_000) + " [1]"
	for index := range 3 {
		params := testBeginAsk("conversation-history", fmt.Sprintf("history-key-%d", index),
			fmt.Sprintf("history-user-%d", index), fmt.Sprintf("history-answer-%d", index), nil)
		params.ExpectedRevision = int64(index)
		params.RequestHash = strings.Repeat(string(rune('d'+index)), 64)
		started, err := database.BeginAsk(t.Context(), params)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.BindAnswerSources(t.Context(), started.AnswerMessageID, hits[:1]); err != nil {
			t.Fatal(err)
		}
		if err := database.CompleteAnswer(t.Context(), started.AnswerMessageID, largeAnswer, []int{1}); err != nil {
			t.Fatal(err)
		}
	}

	conversationPage, err := database.ListConversations(t.Context(), nil, 2)
	if err != nil || len(conversationPage.Items) != 2 || conversationPage.NextCursor == nil {
		t.Fatalf("conversation page one = %#v, %v", conversationPage, err)
	}
	messagePage, err := database.ListConversationMessages(t.Context(), "conversation-history", nil, 2)
	if err != nil || len(messagePage.Items) != 2 || messagePage.NextCursor == nil ||
		messagePage.Items[0].Ordinal != 1 || messagePage.Items[1].Ordinal != 2 ||
		len(messagePage.Items[1].Sources) != 1 || len(messagePage.Items[1].Citations) != 1 {
		t.Fatalf("message page one = %#v, %v", messagePage, err)
	}
	conversationCursor := *conversationPage.NextCursor
	firstConversationIDs := []string{conversationPage.Items[0].ID, conversationPage.Items[1].ID}
	messageCursor := *messagePage.NextCursor

	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(t.Context(), path, Options{BusyTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	reopened.now = clock.Now
	t.Cleanup(func() { _ = reopened.Close() })
	conversationPage, err = reopened.ListConversations(t.Context(), &conversationCursor, 2)
	if err != nil || len(conversationPage.Items) != 2 {
		t.Fatalf("conversation page after restart = %#v, %v", conversationPage, err)
	}
	seenConversations := map[string]struct{}{}
	for _, id := range append(firstConversationIDs, conversationPage.Items[0].ID, conversationPage.Items[1].ID) {
		seenConversations[id] = struct{}{}
	}
	if len(seenConversations) != 4 {
		t.Fatalf("conversation cursor duplicated or skipped rows: %#v", seenConversations)
	}
	messagePage, err = reopened.ListConversationMessages(t.Context(), "conversation-history", &messageCursor, 2)
	if err != nil || len(messagePage.Items) != 2 || messagePage.NextCursor == nil ||
		messagePage.Items[0].Ordinal != 3 || messagePage.Items[1].Ordinal != 4 {
		t.Fatalf("message page after restart = %#v, %v", messagePage, err)
	}
	lastPage, err := reopened.ListConversationMessages(t.Context(), "conversation-history", messagePage.NextCursor, 2)
	if err != nil || len(lastPage.Items) != 2 || lastPage.NextCursor != nil ||
		lastPage.Items[0].Ordinal != 5 || lastPage.Items[1].Ordinal != 6 {
		t.Fatalf("last message page = %#v, %v", lastPage, err)
	}
	bounded, err := reopened.ListConversationMessages(t.Context(), "conversation-history", nil, maxHistoryPageSize)
	if err != nil || bounded.ContentBytes > maxMessagePageBytes || bounded.NextCursor == nil || len(bounded.Items) >= 6 {
		t.Fatalf("content-bounded page = len:%d bytes:%d cursor:%#v err:%v",
			len(bounded.Items), bounded.ContentBytes, bounded.NextCursor, err)
	}
	if _, err := reopened.ListConversationMessages(t.Context(), "missing-conversation", nil, 10); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing conversation history error = %v", err)
	}
	if _, err := reopened.ListConversations(t.Context(), nil, maxHistoryPageSize+1); err == nil {
		t.Fatal("oversized conversation page unexpectedly accepted")
	}
	fakeCursor := HistoryCursor{CreatedAt: testTime, ID: "not-in-this-page-scope"}
	if _, err := reopened.ListConversations(t.Context(), &fakeCursor, 10); !errors.Is(err, ErrHistoryCursorInvalid) {
		t.Fatalf("foreign conversation cursor error = %v", err)
	}
	if _, err := reopened.ListConversationMessages(t.Context(), "conversation-history", &fakeCursor, 10); !errors.Is(err, ErrHistoryCursorInvalid) {
		t.Fatalf("foreign message cursor error = %v", err)
	}
}

func newRAGStore(t *testing.T) *Store {
	t.Helper()
	database, _ := newRAGStoreWithClock(t)
	return database
}

func newRAGStoreWithClock(t *testing.T) (*Store, *fakeClock) {
	t.Helper()
	clock := newFakeClock(testTime)
	database := newTestStore(t, clock)
	if _, err := database.SaveOllamaConfig(t.Context(), SaveOllamaConfigParams{
		ExpectedVersion: 0, Endpoint: "http://127.0.0.1:11434",
		Model: "qwen3:8b", Timeout: time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	seedRAGDocuments(t, database)
	return database, clock
}

func seedRAGDocuments(t *testing.T, database *Store) {
	t.Helper()
	now := testTime.UnixMicro()
	err := database.withTx(t.Context(), func(tx *sql.Tx) error {
		statements := []struct {
			query string
			args  []any
		}{
			{`INSERT INTO documents VALUES (?, ?, ?, ?, ?, ?)`, []any{"document-a", "资料 A", "text/plain", "active", now, now}},
			{`INSERT INTO document_revisions VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, []any{"revision-a", "document-a", 1, "hash-a", nil, 1, now, now}},
			{`INSERT INTO chunks(id, document_id, revision_id, ordinal, content, created_at) VALUES (?, ?, ?, ?, ?, ?)`, []any{"chunk-a", "document-a", "revision-a", 0, "共享知识库问题，来自资料 A。", now}},
			{`INSERT INTO documents VALUES (?, ?, ?, ?, ?, ?)`, []any{"document-b", "资料 B", "text/plain", "active", now, now}},
			{`INSERT INTO document_revisions VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, []any{"revision-b", "document-b", 1, "hash-b", nil, 1, now, now}},
			{`INSERT INTO chunks(id, document_id, revision_id, ordinal, content, created_at) VALUES (?, ?, ?, ?, ?, ?)`, []any{"chunk-b", "document-b", "revision-b", 0, "共享知识库问题，来自资料 B。", now}},
			{`INSERT INTO collections VALUES (?, ?, ?, ?)`, []any{"collection-a", "Collection A", now, now}},
			{`INSERT INTO collections VALUES (?, ?, ?, ?)`, []any{"collection-b", "Collection B", now, now}},
			{`INSERT INTO collection_documents VALUES (?, ?, ?)`, []any{"collection-a", "document-a", now}},
			{`INSERT INTO collection_documents VALUES (?, ?, ?)`, []any{"collection-b", "document-b", now}},
		}
		for _, statement := range statements {
			if _, err := tx.ExecContext(t.Context(), statement.query, statement.args...); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed RAG data: %v", err)
	}
}

func testBeginAsk(conversationID, key, userID, answerID string, scope *string) BeginAskParams {
	return BeginAskParams{
		ConversationID: conversationID, ExpectedRevision: 0,
		IdempotencyKey: key, RequestHash: strings.Repeat("a", 64),
		UserMessageID: userID, AnswerMessageID: answerID,
		Question: "知识库", ScopeCollectionID: scope,
	}
}

func assertCompletedAnswer(t *testing.T, answer Answer, err error, chunkID string, configVersion int64) {
	t.Helper()
	if err != nil || answer.Status != MessageCompleted || answer.ProviderConfigVersion != configVersion ||
		len(answer.Sources) != 1 || answer.Sources[0].ChunkID != chunkID ||
		len(answer.Citations) != 1 || answer.Citations[0].Position != 1 {
		t.Fatalf("completed answer = %#v, %v", answer, err)
	}
}
