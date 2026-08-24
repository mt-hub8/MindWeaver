package sqlite

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestCanonicalConsistencyAcceptsLegalAnswerStates(t *testing.T) {
	database := newRAGStore(t)
	makeAsk := func(conversation, answer string) AskStart {
		t.Helper()
		if _, err := database.CreateConversation(t.Context(), conversation, "History"); err != nil {
			t.Fatal(err)
		}
		started, err := database.BeginAsk(t.Context(), testBeginAsk(
			conversation, "key-"+conversation, "user-"+conversation, answer, nil,
		))
		if err != nil {
			t.Fatal(err)
		}
		return started
	}
	makeAsk("pending-empty", "answer-pending-empty")
	boundPending := makeAsk("pending-bound", "answer-pending-bound")
	bindOneCanonicalSource(t, database, boundPending.AnswerMessageID)
	refused := makeAsk("refused", "answer-refused")
	bindOneCanonicalSource(t, database, refused.AnswerMessageID)
	if err := database.RefuseAnswer(t.Context(), refused.AnswerMessageID, "没有可发布答案。", "NO_HIT"); err != nil {
		t.Fatal(err)
	}
	refusedEmpty := makeAsk("refused-empty", "answer-refused-empty")
	if err := database.RefuseAnswer(t.Context(), refusedEmpty.AnswerMessageID, "没有可发布答案。", "NO_HIT"); err != nil {
		t.Fatal(err)
	}
	failed := makeAsk("failed", "answer-failed")
	bindOneCanonicalSource(t, database, failed.AnswerMessageID)
	if err := database.FailAnswer(t.Context(), failed.AnswerMessageID, "模型不可用。", "MODEL_UNAVAILABLE", "MODEL_UNAVAILABLE"); err != nil {
		t.Fatal(err)
	}
	failedEmpty := makeAsk("failed-empty", "answer-failed-empty")
	if err := database.FailAnswer(t.Context(), failedEmpty.AnswerMessageID, "模型不可用。", "MODEL_UNAVAILABLE", "MODEL_UNAVAILABLE"); err != nil {
		t.Fatal(err)
	}
	completed := makeAsk("completed", "answer-completed")
	bindOneCanonicalSource(t, database, completed.AnswerMessageID)
	if err := database.CompleteAnswer(t.Context(), completed.AnswerMessageID, "答案 [1]。", []int{1}); err != nil {
		t.Fatal(err)
	}
	if err := database.CanonicalConsistencyCheck(t.Context()); err != nil {
		t.Fatalf("legal pending/refused/failed/completed history rejected: %v", err)
	}
}

func TestCanonicalConsistencyAcceptsReconciledOutcomeUncertain(t *testing.T) {
	for _, withSource := range []bool{false, true} {
		name := "without-source"
		if withSource {
			name = "with-source"
		}
		t.Run(name, func(t *testing.T) {
			database := newRAGStore(t)
			started := beginCanonicalAsk(t, database, "uncertain-"+name, "answer-uncertain-"+name)
			if withSource {
				bindOneCanonicalSource(t, database, started.AnswerMessageID)
			}
			if count, err := database.ReconcileAllPendingAnswers(t.Context()); err != nil || count != 1 {
				t.Fatalf("reconcile pending answer = %d, %v", count, err)
			}
			answer, err := database.GetAnswer(t.Context(), started.AnswerMessageID)
			if err != nil || answer.Status != MessageFailed || answer.ErrorCode != "OUTCOME_UNCERTAIN" {
				t.Fatalf("reconciled answer = %+v, %v", answer, err)
			}
			if err := database.CanonicalConsistencyCheck(t.Context()); err != nil {
				t.Fatalf("legal OUTCOME_UNCERTAIN history rejected: %v", err)
			}
		})
	}
}

func TestCanonicalConsistencyRejectsSemanticCorruptionThatSQLiteAccepts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *Store)
	}{
		{
			name: "cross conversation Ask pair",
			mutate: func(t *testing.T, database *Store) {
				started := beginCanonicalAsk(t, database, "conversation-a", "answer-a")
				if _, err := database.CreateConversation(t.Context(), "conversation-b", "Other"); err != nil {
					t.Fatal(err)
				}
				if _, err := database.db.ExecContext(t.Context(), `
					UPDATE conversation_messages SET conversation_id = 'conversation-b'
					WHERE id = ?
				`, started.AnswerMessageID); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "unbound message",
			mutate: func(t *testing.T, database *Store) {
				beginCanonicalAsk(t, database, "conversation", "answer")
				now := testTime.AddDate(0, 0, 1).UnixMicro()
				if _, err := database.db.ExecContext(t.Context(), `
					INSERT INTO conversation_messages(
						id, conversation_id, ordinal, role, status, content,
						provider_config_version, limitation_code, error_code,
						created_at, completed_at, reconcile_after
					) VALUES ('orphan-user', 'conversation', 3, 'user', 'completed',
						'orphan', NULL, NULL, NULL, ?, ?, NULL)
				`, now, now); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "conversation revision mismatch",
			mutate: func(t *testing.T, database *Store) {
				beginCanonicalAsk(t, database, "conversation", "answer")
				if _, err := database.db.ExecContext(t.Context(), `
					UPDATE conversations SET revision = revision + 1 WHERE id = 'conversation'
				`); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "multiple pending answers",
			mutate: func(t *testing.T, database *Store) {
				beginCanonicalAsk(t, database, "conversation", "answer")
				now := testTime.AddDate(0, 0, 1).UnixMicro()
				if _, err := database.db.ExecContext(t.Context(), `
					INSERT INTO conversation_messages(
						id, conversation_id, ordinal, role, status, content,
						provider_config_version, limitation_code, error_code,
						created_at, completed_at, reconcile_after
					) VALUES ('second-pending', 'conversation', 3, 'assistant', 'pending', '',
						1, NULL, NULL, ?, NULL, ?)
				`, now, now+1); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "source positions start at two",
			mutate: func(t *testing.T, database *Store) {
				started := beginCanonicalAsk(t, database, "conversation", "answer")
				bindOneCanonicalSource(t, database, started.AnswerMessageID)
				if _, err := database.db.ExecContext(t.Context(), `
					UPDATE answer_sources SET source_position = 2 WHERE answer_message_id = ?
				`, started.AnswerMessageID); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "completed answer without evidence",
			mutate: func(t *testing.T, database *Store) {
				started := beginCanonicalAsk(t, database, "conversation", "answer")
				now := testTime.AddDate(0, 0, 1).UnixMicro()
				if _, err := database.db.ExecContext(t.Context(), `
					UPDATE conversation_messages
					SET status = 'completed', content = 'unsupported answer', completed_at = ?
					WHERE id = ?
				`, now, started.AnswerMessageID); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "pending answer with citation",
			mutate: func(t *testing.T, database *Store) {
				started := beginCanonicalAsk(t, database, "conversation", "answer")
				bindOneCanonicalSource(t, database, started.AnswerMessageID)
				if _, err := database.db.ExecContext(t.Context(), `
					INSERT INTO answer_citations(answer_message_id, occurrence, source_position)
					VALUES (?, 1, 1)
				`, started.AnswerMessageID); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "wrong but well formed source hash",
			mutate: func(t *testing.T, database *Store) {
				started := beginCanonicalAsk(t, database, "conversation", "answer")
				bindOneCanonicalSource(t, database, started.AnswerMessageID)
				if _, err := database.db.ExecContext(t.Context(), `
					UPDATE answer_sources SET content_hash = ? WHERE answer_message_id = ?
				`, strings.Repeat("f", 64), started.AnswerMessageID); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "oversized source content",
			mutate: func(t *testing.T, database *Store) {
				started := beginCanonicalAsk(t, database, "conversation", "answer")
				bindOneCanonicalSource(t, database, started.AnswerMessageID)
				if _, err := database.db.ExecContext(t.Context(), `
					UPDATE chunks SET content = ? WHERE id = 'chunk-a'
				`, strings.Repeat("x", maxAnswerSourceBytes+1)); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "whitespace-only source content",
			mutate: func(t *testing.T, database *Store) {
				started := beginCanonicalAsk(t, database, "conversation", "answer")
				bindOneCanonicalSource(t, database, started.AnswerMessageID)
				content := "\t\r\n"
				digest := sha256.Sum256([]byte(content))
				if _, err := database.db.ExecContext(t.Context(), `
					UPDATE chunks SET content = ? WHERE id = 'chunk-a'
				`, content); err != nil {
					t.Fatal(err)
				}
				if _, err := database.db.ExecContext(t.Context(), `
					UPDATE answer_sources SET content_hash = ? WHERE answer_message_id = ?
				`, hex.EncodeToString(digest[:]), started.AnswerMessageID); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			database := newRAGStore(t)
			test.mutate(t, database)
			if err := database.IntegrityCheck(t.Context()); err != nil {
				t.Fatalf("fixture must remain SQLite/FK/FTS valid: %v", err)
			}
			if err := database.CanonicalConsistencyCheck(t.Context()); !errors.Is(err, ErrCanonicalConsistency) {
				t.Fatalf("canonical consistency error = %v, want ErrCanonicalConsistency", err)
			}
		})
	}
}

func TestCanonicalSourceContentBoundRejectsBeforeContentPaging(t *testing.T) {
	database := newRAGStore(t)
	started := beginCanonicalAsk(t, database, "oversized-preflight", "answer-oversized-preflight")
	bindOneCanonicalSource(t, database, started.AnswerMessageID)
	if _, err := database.db.ExecContext(t.Context(), `
		UPDATE chunks SET content = ? WHERE id = 'chunk-a'
	`, strings.Repeat("x", maxAnswerSourceBytes+1)); err != nil {
		t.Fatal(err)
	}
	tx, err := database.db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := checkCanonicalSourceContentBounds(t.Context(), tx); !errors.Is(err, ErrCanonicalConsistency) {
		t.Fatalf("bounded preflight error = %v, want ErrCanonicalConsistency", err)
	}
}

func TestCanonicalSourceContentRejectsAggregateBeyondAdmissionLimit(t *testing.T) {
	database := newRAGStore(t)
	started := beginCanonicalAsk(t, database, "aggregate-source-limit", "answer-aggregate-source-limit")
	hits, err := database.Search(t.Context(), "共享知识库问题", 2)
	if err != nil || len(hits) != 2 {
		t.Fatalf("aggregate source search = %#v, %v", hits, err)
	}
	if _, err := database.BindAnswerSources(t.Context(), started.AnswerMessageID, hits); err != nil {
		t.Fatal(err)
	}
	content := strings.Repeat("x", maxAnswerSourceBytes/2+1)
	digest := sha256.Sum256([]byte(content))
	contentHash := hex.EncodeToString(digest[:])
	for _, hit := range hits {
		if _, err := database.db.ExecContext(t.Context(), `
			UPDATE chunks SET content = ? WHERE id = ?
		`, content, hit.ChunkID); err != nil {
			t.Fatal(err)
		}
		if _, err := database.db.ExecContext(t.Context(), `
			UPDATE answer_sources SET content_hash = ?
			WHERE answer_message_id = ? AND chunk_id = ?
		`, contentHash, started.AnswerMessageID, hit.ChunkID); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.IntegrityCheck(t.Context()); err != nil {
		t.Fatalf("aggregate fixture must remain SQLite/FK/FTS valid: %v", err)
	}
	if err := database.CanonicalConsistencyCheck(t.Context()); !errors.Is(err, ErrCanonicalConsistency) {
		t.Fatalf("aggregate source consistency error = %v, want ErrCanonicalConsistency", err)
	}
}

func beginCanonicalAsk(t *testing.T, database *Store, conversation, answer string) AskStart {
	t.Helper()
	if _, err := database.CreateConversation(t.Context(), conversation, "History"); err != nil {
		t.Fatal(err)
	}
	started, err := database.BeginAsk(t.Context(), testBeginAsk(
		conversation, "key-"+conversation, fmt.Sprintf("user-%s", conversation), answer, nil,
	))
	if err != nil {
		t.Fatal(err)
	}
	return started
}

func bindOneCanonicalSource(t *testing.T, database *Store, answerID string) {
	t.Helper()
	hits, err := database.Search(t.Context(), "知识库", 1)
	if err != nil || len(hits) != 1 {
		t.Fatalf("canonical source search = %#v, %v", hits, err)
	}
	if _, err := database.BindAnswerSources(t.Context(), answerID, hits); err != nil {
		t.Fatal(err)
	}
}
