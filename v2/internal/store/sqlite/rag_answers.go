package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	maxOllamaEndpointBytes   = 2048
	maxOllamaModelBytes      = 255
	maxConversationTitle     = 1024
	maxAskQuestionBytes      = 1024
	maxAnswerBytes           = 256 << 10
	maxAnswerSources         = 8
	maxAnswerSourceBytes     = 48 << 10
	maxAnswerCitations       = 100
	answerReconcileGrace     = 30 * time.Second
	maxHistoryPageSize       = 50
	maxMessagePageBytes      = 512 << 10
	maxPendingReconcileBatch = 100

	// These are controlled product strings. Provider output is never persisted
	// when a call fails or its citations cannot be structurally verified.
	OutcomeUncertainText = "应用上次中断时无法确认模型调用结果；为避免重复调用，本次回答已停止。"
	SourceChangedText    = "回答生成期间资料范围发生变化，本次回答未发布。"
)

var (
	ErrConversationRevision    = errors.New("sqlite: conversation revision conflict")
	ErrConversationIdempotency = errors.New("sqlite: conversation idempotency key was already used for a different title")
	ErrConversationBusy        = errors.New("sqlite: conversation has a pending answer")
	ErrProviderConfigRevision  = errors.New("sqlite: Ollama configuration revision conflict")
	ErrAskIdempotency          = errors.New("sqlite: ask idempotency key was already used for a different request")
	ErrOllamaNotConfigured     = errors.New("sqlite: Ollama is not configured")
	ErrAnswerNotPending        = errors.New("sqlite: answer is not pending")
	ErrAnswerSourceChanged     = errors.New("sqlite: answer source changed or left scope")
	ErrInvalidCitation         = errors.New("sqlite: citation does not reference a supplied source")
	ErrHistoryItemTooLarge     = errors.New("sqlite: history item exceeds response content limit")
	ErrHistoryCursorInvalid    = errors.New("sqlite: history cursor does not identify an item in scope")
)

// MessageStatus is the small durable state machine for a user-visible Ask.
type MessageStatus string

const (
	MessagePending   MessageStatus = "pending"
	MessageCompleted MessageStatus = "completed"
	MessageRefused   MessageStatus = "refused"
	MessageFailed    MessageStatus = "failed"
)

// OllamaConfig is one immutable concrete local-model configuration version.
// There is deliberately no credential field or generic provider registry.
type OllamaConfig struct {
	Version   int64
	Endpoint  string
	Model     string
	Timeout   time.Duration
	Active    bool
	CreatedAt time.Time
}

type SaveOllamaConfigParams struct {
	ExpectedVersion int64
	Endpoint        string
	Model           string
	Timeout         time.Duration
}

// SaveOllamaConfig appends one immutable version and atomically makes it the
// active concrete Ollama selection. ExpectedVersion is zero for first setup.
func (s *Store) SaveOllamaConfig(ctx context.Context, params SaveOllamaConfigParams) (OllamaConfig, error) {
	if err := validateOllamaConfig(params); err != nil {
		return OllamaConfig{}, err
	}
	var result OllamaConfig
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		var activeVersion int64
		err := tx.QueryRowContext(ctx, `
			SELECT config_version FROM ollama_config_versions WHERE is_active = 1
		`).Scan(&activeVersion)
		if errors.Is(err, sql.ErrNoRows) {
			activeVersion = 0
		} else if err != nil {
			return fmt.Errorf("read active Ollama configuration: %w", err)
		}
		if activeVersion != params.ExpectedVersion {
			return ErrProviderConfigRevision
		}

		var nextVersion int64
		if err := tx.QueryRowContext(ctx, `
			SELECT COALESCE(max(config_version), 0) + 1 FROM ollama_config_versions
		`).Scan(&nextVersion); err != nil {
			return fmt.Errorf("allocate Ollama configuration version: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE ollama_config_versions SET is_active = 0 WHERE is_active = 1
		`); err != nil {
			return fmt.Errorf("deactivate Ollama configuration: %w", err)
		}
		now := s.nowMicros()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO ollama_config_versions(
				config_version, endpoint, model, timeout_milliseconds, is_active, created_at
			) VALUES (?, ?, ?, ?, 1, ?)
		`, nextVersion, params.Endpoint, params.Model, params.Timeout.Milliseconds(), now); err != nil {
			return fmt.Errorf("insert Ollama configuration: %w", err)
		}
		result = OllamaConfig{
			Version: nextVersion, Endpoint: params.Endpoint, Model: params.Model,
			Timeout: params.Timeout, Active: true, CreatedAt: time.UnixMicro(now).UTC(),
		}
		return nil
	})
	if err != nil {
		return OllamaConfig{}, err
	}
	return result, nil
}

func (s *Store) GetActiveOllamaConfig(ctx context.Context) (OllamaConfig, error) {
	config, err := scanOllamaConfig(s.db.QueryRowContext(ctx, ollamaConfigSelect+" WHERE is_active = 1"))
	if errors.Is(err, sql.ErrNoRows) {
		return OllamaConfig{}, ErrOllamaNotConfigured
	}
	if err != nil {
		return OllamaConfig{}, fmt.Errorf("sqlite: get active Ollama configuration: %w", err)
	}
	return config, nil
}

func (s *Store) GetOllamaConfig(ctx context.Context, version int64) (OllamaConfig, error) {
	if version < 1 {
		return OllamaConfig{}, errors.New("sqlite: Ollama configuration version must be positive")
	}
	config, err := scanOllamaConfig(s.db.QueryRowContext(ctx, ollamaConfigSelect+" WHERE config_version = ?", version))
	if errors.Is(err, sql.ErrNoRows) {
		return OllamaConfig{}, ErrNotFound
	}
	if err != nil {
		return OllamaConfig{}, fmt.Errorf("sqlite: get Ollama configuration: %w", err)
	}
	return config, nil
}

const ollamaConfigSelect = `
	SELECT config_version, endpoint, model, timeout_milliseconds, is_active, created_at
	FROM ollama_config_versions`

func scanOllamaConfig(row scanner) (OllamaConfig, error) {
	var config OllamaConfig
	var timeoutMillis, createdAt int64
	var active int
	if err := row.Scan(&config.Version, &config.Endpoint, &config.Model, &timeoutMillis, &active, &createdAt); err != nil {
		return OllamaConfig{}, err
	}
	config.Timeout = time.Duration(timeoutMillis) * time.Millisecond
	config.Active = active == 1
	config.CreatedAt = time.UnixMicro(createdAt).UTC()
	return config, nil
}

// Conversation uses one optimistic revision per accepted Ask pair. This is a
// browser concurrency root, not a message-count approximation.
type Conversation struct {
	ID              string
	Title           string
	Revision        int64
	PendingAnswer   bool
	PendingAnswerID *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// HistoryCursor is an exact stable database key. Conversation pages sort it
// descending; message pages sort it ascending.
type HistoryCursor struct {
	CreatedAt time.Time
	ID        string
}

type ConversationPage struct {
	Items      []Conversation
	NextCursor *HistoryCursor
}

func (s *Store) CreateConversation(ctx context.Context, id, title string) (Conversation, error) {
	if err := validateIdentifier("conversation id", id); err != nil {
		return Conversation{}, err
	}
	if !validDisplayText(title, maxConversationTitle) {
		return Conversation{}, fmt.Errorf("sqlite: conversation title must contain 1 to %d UTF-8 bytes", maxConversationTitle)
	}
	now := s.nowMicros()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO conversations(id, title, revision, created_at, updated_at)
		VALUES (?, ?, 0, ?, ?)
	`, id, title, now, now); err != nil {
		return Conversation{}, fmt.Errorf("sqlite: create conversation: %w", err)
	}
	stamp := time.UnixMicro(now).UTC()
	return Conversation{ID: id, Title: title, Revision: 0, CreatedAt: stamp, UpdatedAt: stamp}, nil
}

// CreateConversationIdempotent creates the caller-derived conversation
// identity once. Reusing that identity with the same title returns the
// original row; reusing it for a different title is a conflict.
func (s *Store) CreateConversationIdempotent(ctx context.Context, id, title string) (Conversation, bool, error) {
	if err := validateIdentifier("conversation id", id); err != nil {
		return Conversation{}, false, err
	}
	if !validDisplayText(title, maxConversationTitle) {
		return Conversation{}, false, fmt.Errorf("sqlite: conversation title must contain 1 to %d UTF-8 bytes", maxConversationTitle)
	}
	var conversation Conversation
	created := false
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.nowMicros()
		result, err := tx.ExecContext(ctx, `
			INSERT INTO conversations(id, title, revision, created_at, updated_at)
			VALUES (?, ?, 0, ?, ?)
			ON CONFLICT(id) DO NOTHING
		`, id, title, now, now)
		if err != nil {
			return fmt.Errorf("create idempotent conversation: %w", err)
		}
		count, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("count idempotent conversation creation: %w", err)
		}
		created = count == 1
		var createdAt, updatedAt int64
		var pendingAnswerID sql.NullString
		if err := tx.QueryRowContext(ctx, `
			SELECT c.id, c.title, c.revision, c.created_at, c.updated_at,
				(SELECT m.id FROM conversation_messages AS m
					WHERE m.conversation_id = c.id AND m.role = 'assistant' AND m.status = 'pending'
					ORDER BY m.created_at, m.id LIMIT 1)
			FROM conversations AS c WHERE c.id = ?
		`, id).Scan(&conversation.ID, &conversation.Title, &conversation.Revision, &createdAt, &updatedAt, &pendingAnswerID); err != nil {
			return fmt.Errorf("read idempotent conversation: %w", err)
		}
		if conversation.Title != title {
			return ErrConversationIdempotency
		}
		conversation.CreatedAt = time.UnixMicro(createdAt).UTC()
		conversation.UpdatedAt = time.UnixMicro(updatedAt).UTC()
		setPendingAnswerProjection(&conversation, pendingAnswerID)
		return nil
	})
	if err != nil {
		return Conversation{}, false, err
	}
	return conversation, created, nil
}

func (s *Store) GetConversation(ctx context.Context, id string) (Conversation, error) {
	if err := validateIdentifier("conversation id", id); err != nil {
		return Conversation{}, err
	}
	var conversation Conversation
	var createdAt, updatedAt int64
	var pendingAnswerID sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT c.id, c.title, c.revision, c.created_at, c.updated_at,
			(SELECT m.id FROM conversation_messages AS m
				WHERE m.conversation_id = c.id AND m.role = 'assistant' AND m.status = 'pending'
				ORDER BY m.created_at, m.id LIMIT 1)
		FROM conversations AS c WHERE c.id = ?
	`, id).Scan(&conversation.ID, &conversation.Title, &conversation.Revision, &createdAt, &updatedAt, &pendingAnswerID)
	if errors.Is(err, sql.ErrNoRows) {
		return Conversation{}, ErrNotFound
	}
	if err != nil {
		return Conversation{}, fmt.Errorf("sqlite: get conversation: %w", err)
	}
	conversation.CreatedAt = time.UnixMicro(createdAt).UTC()
	conversation.UpdatedAt = time.UnixMicro(updatedAt).UTC()
	setPendingAnswerProjection(&conversation, pendingAnswerID)
	return conversation, nil
}

// ListConversations returns a bounded newest-first page ordered by the stable
// tuple (created_at, id). Titles cap the page's user-content payload at 50 KiB.
func (s *Store) ListConversations(ctx context.Context, after *HistoryCursor, limit int) (ConversationPage, error) {
	if err := validateHistoryPage(after, limit); err != nil {
		return ConversationPage{}, err
	}
	if ctx == nil {
		return ConversationPage{}, errors.New("sqlite: nil context")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ConversationPage{}, fmt.Errorf("sqlite: begin conversation history read: %w", err)
	}
	defer tx.Rollback()
	query := `
		SELECT c.id, c.title, c.revision, c.created_at, c.updated_at,
			(SELECT m.id FROM conversation_messages AS m
				WHERE m.conversation_id = c.id AND m.role = 'assistant' AND m.status = 'pending'
				ORDER BY m.created_at, m.id LIMIT 1)
		FROM conversations AS c`
	args := make([]any, 0, 4)
	if after != nil {
		stamp := after.CreatedAt.UnixMicro()
		var cursorExists int
		if err := tx.QueryRowContext(ctx, `
			SELECT 1 FROM conversations WHERE id = ? AND created_at = ?
		`, after.ID, stamp).Scan(&cursorExists); errors.Is(err, sql.ErrNoRows) {
			return ConversationPage{}, ErrHistoryCursorInvalid
		} else if err != nil {
			return ConversationPage{}, fmt.Errorf("sqlite: validate conversation cursor: %w", err)
		}
		query += ` WHERE created_at < ? OR (created_at = ? AND id < ?)`
		args = append(args, stamp, stamp, after.ID)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return ConversationPage{}, fmt.Errorf("sqlite: list conversations: %w", err)
	}
	items := make([]Conversation, 0, limit+1)
	for rows.Next() {
		conversation, err := scanConversation(rows)
		if err != nil {
			_ = rows.Close()
			return ConversationPage{}, fmt.Errorf("sqlite: scan conversation page: %w", err)
		}
		items = append(items, conversation)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return ConversationPage{}, fmt.Errorf("sqlite: iterate conversation page: %w", err)
	}
	if err := rows.Close(); err != nil {
		return ConversationPage{}, fmt.Errorf("sqlite: close conversation page: %w", err)
	}
	page := ConversationPage{Items: items}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		last := page.Items[len(page.Items)-1]
		page.NextCursor = &HistoryCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	if err := tx.Commit(); err != nil {
		return ConversationPage{}, fmt.Errorf("sqlite: finish conversation history read: %w", err)
	}
	return page, nil
}

func scanConversation(row scanner) (Conversation, error) {
	var conversation Conversation
	var createdAt, updatedAt int64
	var pendingAnswerID sql.NullString
	if err := row.Scan(&conversation.ID, &conversation.Title, &conversation.Revision, &createdAt, &updatedAt, &pendingAnswerID); err != nil {
		return Conversation{}, err
	}
	conversation.CreatedAt = time.UnixMicro(createdAt).UTC()
	conversation.UpdatedAt = time.UnixMicro(updatedAt).UTC()
	setPendingAnswerProjection(&conversation, pendingAnswerID)
	return conversation, nil
}

func setPendingAnswerProjection(conversation *Conversation, pending sql.NullString) {
	conversation.PendingAnswer = pending.Valid
	conversation.PendingAnswerID = nil
	if pending.Valid {
		value := pending.String
		conversation.PendingAnswerID = &value
	}
}

// DeleteConversation is the explicit user action that removes a conversation,
// its Ask pairs, frozen sources, and citations. Missing is an idempotent
// deleted=false result; a present but stale revision is a conflict. This is the
// only safe way to release answer provenance before retrying document purge.
func (s *Store) DeleteConversation(ctx context.Context, id string, expectedRevision int64) (bool, error) {
	if err := validateIdentifier("conversation id", id); err != nil {
		return false, err
	}
	if expectedRevision < 0 {
		return false, errors.New("sqlite: expected conversation revision must not be negative")
	}
	deleted := false
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		var currentRevision int64
		err := tx.QueryRowContext(ctx, `
			SELECT revision FROM conversations WHERE id = ?
		`, id).Scan(&currentRevision)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read conversation before deletion: %w", err)
		}
		if currentRevision != expectedRevision {
			return ErrConversationRevision
		}
		var pending int
		if err := tx.QueryRowContext(ctx, `
			SELECT count(*) FROM conversation_messages
			WHERE conversation_id = ? AND role = 'assistant' AND status = 'pending'
		`, id).Scan(&pending); err != nil {
			return fmt.Errorf("inspect pending answers before conversation deletion: %w", err)
		}
		if pending != 0 {
			return ErrConversationBusy
		}
		result, err := tx.ExecContext(ctx, `
			DELETE FROM conversations WHERE id = ? AND revision = ?
		`, id, expectedRevision)
		if err != nil {
			return fmt.Errorf("delete conversation: %w", err)
		}
		count, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("count deleted conversations: %w", err)
		}
		if count != 1 {
			return ErrConversationRevision
		}
		deleted = true
		return nil
	})
	return deleted, err
}

type ConversationMessage struct {
	ID                    string
	ConversationID        string
	Ordinal               int
	Role                  string
	Status                MessageStatus
	Content               string
	ProviderConfigVersion int64
	ScopeCollectionID     *string
	LimitationCode        string
	ErrorCode             string
	CreatedAt             time.Time
	CompletedAt           *time.Time
	ReconcileAfter        *time.Time
	Sources               []AnswerSource
	Citations             []Citation
}

type ConversationMessagePage struct {
	Items        []ConversationMessage
	NextCursor   *HistoryCursor
	ContentBytes int
}

// ListConversationMessages returns oldest-first history for one conversation.
// The stable cursor is (created_at,id), the page has at most 50 rows, and its
// combined message, source, and display-title content is at most 512 KiB.
func (s *Store) ListConversationMessages(ctx context.Context, conversationID string, after *HistoryCursor, limit int) (ConversationMessagePage, error) {
	if err := validateIdentifier("conversation id", conversationID); err != nil {
		return ConversationMessagePage{}, err
	}
	if err := validateHistoryPage(after, limit); err != nil {
		return ConversationMessagePage{}, err
	}
	if ctx == nil {
		return ConversationMessagePage{}, errors.New("sqlite: nil context")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ConversationMessagePage{}, fmt.Errorf("sqlite: begin message history read: %w", err)
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `
		SELECT 1 FROM conversations WHERE id = ?
	`, conversationID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return ConversationMessagePage{}, ErrNotFound
	} else if err != nil {
		return ConversationMessagePage{}, fmt.Errorf("sqlite: inspect conversation history: %w", err)
	}
	query := `
		SELECT m.id, m.conversation_id, m.ordinal, m.role, m.status, m.content,
			m.provider_config_version, COALESCE(ua.scope_collection_id, aa.scope_collection_id),
			m.limitation_code, m.error_code, m.created_at, m.completed_at, m.reconcile_after
		FROM conversation_messages AS m
		LEFT JOIN ask_requests AS ua ON ua.user_message_id = m.id
		LEFT JOIN ask_requests AS aa ON aa.answer_message_id = m.id
		WHERE m.conversation_id = ?`
	args := []any{conversationID}
	if after != nil {
		stamp := after.CreatedAt.UnixMicro()
		var cursorExists int
		if err := tx.QueryRowContext(ctx, `
			SELECT 1 FROM conversation_messages
			WHERE conversation_id = ? AND id = ? AND created_at = ?
		`, conversationID, after.ID, stamp).Scan(&cursorExists); errors.Is(err, sql.ErrNoRows) {
			return ConversationMessagePage{}, ErrHistoryCursorInvalid
		} else if err != nil {
			return ConversationMessagePage{}, fmt.Errorf("sqlite: validate message cursor: %w", err)
		}
		query += ` AND (m.created_at > ? OR (m.created_at = ? AND m.id > ?))`
		args = append(args, stamp, stamp, after.ID)
	}
	query += ` ORDER BY m.created_at, m.id LIMIT ?`
	args = append(args, limit+1)
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return ConversationMessagePage{}, fmt.Errorf("sqlite: list conversation messages: %w", err)
	}
	candidates := make([]ConversationMessage, 0, limit+1)
	for rows.Next() {
		message, err := scanConversationMessage(rows)
		if err != nil {
			_ = rows.Close()
			return ConversationMessagePage{}, fmt.Errorf("sqlite: scan conversation message: %w", err)
		}
		candidates = append(candidates, message)
	}
	if err := rows.Close(); err != nil {
		return ConversationMessagePage{}, fmt.Errorf("sqlite: close conversation messages: %w", err)
	}
	if err := rows.Err(); err != nil {
		return ConversationMessagePage{}, fmt.Errorf("sqlite: iterate conversation messages: %w", err)
	}

	page := ConversationMessagePage{Items: make([]ConversationMessage, 0, min(limit, len(candidates)))}
	more := len(candidates) > limit
	for index := 0; index < len(candidates) && index < limit; index++ {
		message := candidates[index]
		if message.Role == "assistant" {
			message.Sources, message.Citations, err = readAnswerEvidence(ctx, tx, message.ID)
			if err != nil {
				return ConversationMessagePage{}, err
			}
		}
		itemBytes := historyMessageBytes(message)
		if itemBytes > maxMessagePageBytes {
			return ConversationMessagePage{}, ErrHistoryItemTooLarge
		}
		if page.ContentBytes+itemBytes > maxMessagePageBytes && len(page.Items) != 0 {
			more = true
			break
		}
		page.Items = append(page.Items, message)
		page.ContentBytes += itemBytes
	}
	if more && len(page.Items) != 0 {
		last := page.Items[len(page.Items)-1]
		page.NextCursor = &HistoryCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	if err := tx.Commit(); err != nil {
		return ConversationMessagePage{}, fmt.Errorf("sqlite: finish message history read: %w", err)
	}
	return page, nil
}

func scanConversationMessage(row scanner) (ConversationMessage, error) {
	var message ConversationMessage
	var status string
	var providerVersion, completedAt, reconcileAfter sql.NullInt64
	var scope, limitation, errorCode sql.NullString
	var createdAt int64
	if err := row.Scan(&message.ID, &message.ConversationID, &message.Ordinal,
		&message.Role, &status, &message.Content, &providerVersion,
		&scope, &limitation, &errorCode, &createdAt, &completedAt, &reconcileAfter); err != nil {
		return ConversationMessage{}, err
	}
	message.Status = MessageStatus(status)
	message.ProviderConfigVersion = providerVersion.Int64
	message.LimitationCode = limitation.String
	message.ErrorCode = errorCode.String
	if scope.Valid {
		value := scope.String
		message.ScopeCollectionID = &value
	}
	message.CreatedAt = time.UnixMicro(createdAt).UTC()
	if completedAt.Valid {
		value := time.UnixMicro(completedAt.Int64).UTC()
		message.CompletedAt = &value
	}
	if reconcileAfter.Valid {
		value := time.UnixMicro(reconcileAfter.Int64).UTC()
		message.ReconcileAfter = &value
	}
	return message, nil
}

func historyMessageBytes(message ConversationMessage) int {
	total := len(message.ID) + len(message.ConversationID) + len(message.Role) +
		len(message.Status) + len(message.Content) + len(message.LimitationCode) + len(message.ErrorCode)
	if message.ScopeCollectionID != nil {
		total += len(*message.ScopeCollectionID)
	}
	for _, source := range message.Sources {
		total += len(source.ChunkID) + len(source.DocumentID) + len(source.RevisionID) +
			len(source.ContentHash) + len(source.DocumentTitle) + len(source.Content)
	}
	return total
}

type BeginAskParams struct {
	ConversationID    string
	ExpectedRevision  int64
	IdempotencyKey    string
	RequestHash       string
	UserMessageID     string
	AnswerMessageID   string
	Question          string
	ScopeCollectionID *string
}

// AskStart is the durable reservation made before retrieval or provider I/O.
type AskStart struct {
	Created          bool
	UserMessageID    string
	AnswerMessageID  string
	ProviderConfig   OllamaConfig
	AcceptedRevision int64
}

// BeginAsk atomically checks idempotency and the conversation revision, binds
// the active provider version, writes the completed user message and pending
// assistant message, and advances the conversation revision.
func (s *Store) BeginAsk(ctx context.Context, params BeginAskParams) (AskStart, error) {
	if err := validateBeginAsk(params); err != nil {
		return AskStart{}, err
	}
	var result AskStart
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		var appliedHash, userID, answerID string
		var configVersion, acceptedExpectedRevision int64
		err := tx.QueryRowContext(ctx, `
			SELECT a.request_hash, a.user_message_id, a.answer_message_id,
				m.provider_config_version, a.expected_revision
			FROM ask_requests AS a
			JOIN conversation_messages AS m ON m.id = a.answer_message_id
			WHERE a.conversation_id = ? AND a.idempotency_key = ?
		`, params.ConversationID, params.IdempotencyKey).Scan(
			&appliedHash, &userID, &answerID, &configVersion, &acceptedExpectedRevision,
		)
		switch {
		case err == nil:
			if appliedHash != params.RequestHash {
				return ErrAskIdempotency
			}
			config, err := scanOllamaConfig(tx.QueryRowContext(ctx,
				ollamaConfigSelect+" WHERE config_version = ?", configVersion))
			if err != nil {
				return fmt.Errorf("read replayed Ollama configuration: %w", err)
			}
			result = AskStart{
				Created: false, UserMessageID: userID, AnswerMessageID: answerID,
				ProviderConfig: config, AcceptedRevision: acceptedExpectedRevision + 1,
			}
			return nil
		case !errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("inspect Ask idempotency key: %w", err)
		}

		var currentRevision, maxOrdinal, maxMessageCreatedAt int64
		if err := tx.QueryRowContext(ctx, `
			SELECT revision FROM conversations WHERE id = ?
		`, params.ConversationID).Scan(&currentRevision); errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return fmt.Errorf("read conversation revision: %w", err)
		}
		var pending int
		if err := tx.QueryRowContext(ctx, `
			SELECT count(*) FROM conversation_messages
			WHERE conversation_id = ? AND role = 'assistant' AND status = 'pending'
		`, params.ConversationID).Scan(&pending); err != nil {
			return fmt.Errorf("inspect pending answer before Ask: %w", err)
		}
		if pending != 0 {
			return ErrConversationBusy
		}
		if currentRevision != params.ExpectedRevision {
			return ErrConversationRevision
		}
		config, err := scanOllamaConfig(tx.QueryRowContext(ctx, ollamaConfigSelect+" WHERE is_active = 1"))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrOllamaNotConfigured
		}
		if err != nil {
			return fmt.Errorf("read selected Ollama configuration: %w", err)
		}
		if err := tx.QueryRowContext(ctx, `
			SELECT COALESCE(max(ordinal), 0), COALESCE(max(created_at), -1)
			FROM conversation_messages
			WHERE conversation_id = ?
		`, params.ConversationID).Scan(&maxOrdinal, &maxMessageCreatedAt); err != nil {
			return fmt.Errorf("read conversation message ordinal: %w", err)
		}
		userCreatedAt := s.nowMicros()
		if userCreatedAt <= maxMessageCreatedAt {
			if maxMessageCreatedAt == int64(^uint64(0)>>1) {
				return errors.New("conversation message timestamp exhausted")
			}
			userCreatedAt = maxMessageCreatedAt + 1
		}
		if userCreatedAt == int64(^uint64(0)>>1) {
			return errors.New("conversation message timestamp exhausted")
		}
		answerCreatedAt := userCreatedAt + 1
		reconcileAfter, err := addAnswerReconcileDeadline(answerCreatedAt, config.Timeout)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO conversation_messages(
				id, conversation_id, ordinal, role, status, content,
				provider_config_version, limitation_code, error_code,
				created_at, completed_at, reconcile_after
			) VALUES (?, ?, ?, 'user', 'completed', ?, NULL, NULL, NULL, ?, ?, NULL)
		`, params.UserMessageID, params.ConversationID, maxOrdinal+1, params.Question,
			userCreatedAt, userCreatedAt); err != nil {
			return fmt.Errorf("insert user message: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO conversation_messages(
				id, conversation_id, ordinal, role, status, content,
				provider_config_version, limitation_code, error_code,
				created_at, completed_at, reconcile_after
			) VALUES (?, ?, ?, 'assistant', 'pending', '', ?, NULL, NULL, ?, NULL, ?)
		`, params.AnswerMessageID, params.ConversationID, maxOrdinal+2,
			config.Version, answerCreatedAt, reconcileAfter); err != nil {
			return fmt.Errorf("insert pending answer: %w", err)
		}
		var scope any
		if params.ScopeCollectionID != nil {
			scope = *params.ScopeCollectionID
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO ask_requests(
				conversation_id, idempotency_key, request_hash, expected_revision,
				user_message_id, answer_message_id, scope_collection_id, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, params.ConversationID, params.IdempotencyKey, params.RequestHash,
			params.ExpectedRevision, params.UserMessageID, params.AnswerMessageID,
			scope, userCreatedAt); err != nil {
			return fmt.Errorf("insert Ask request: %w", err)
		}
		updated, err := tx.ExecContext(ctx, `
			UPDATE conversations SET revision = revision + 1, updated_at = max(?, updated_at)
			WHERE id = ? AND revision = ?
		`, answerCreatedAt, params.ConversationID, params.ExpectedRevision)
		if err != nil {
			return fmt.Errorf("advance conversation revision: %w", err)
		}
		count, err := updated.RowsAffected()
		if err != nil || count != 1 {
			return ErrConversationRevision
		}
		result = AskStart{
			Created: true, UserMessageID: params.UserMessageID,
			AnswerMessageID: params.AnswerMessageID, ProviderConfig: config,
			AcceptedRevision: params.ExpectedRevision + 1,
		}
		return nil
	})
	if err != nil {
		return AskStart{}, err
	}
	return result, nil
}

// AnswerSource is the exact ordered source identity frozen before provider I/O.
// Content is intentionally not duplicated into the answer tables.
type AnswerSource struct {
	Position      int
	ChunkID       string
	DocumentID    string
	RevisionID    string
	ChunkOrdinal  int
	ContentHash   string
	DocumentTitle string
	// Content is joined from the authoritative chunks table when an answer is
	// read. It is never duplicated into answer_sources.
	Content string
}

type Citation struct {
	Occurrence int
	Position   int
}

type Answer struct {
	ID                    string
	ConversationID        string
	ConversationRevision  int64
	Question              string
	Status                MessageStatus
	Content               string
	ProviderConfigVersion int64
	ScopeCollectionID     *string
	LimitationCode        string
	ErrorCode             string
	CreatedAt             time.Time
	CompletedAt           *time.Time
	ReconcileAfter        time.Time
	Sources               []AnswerSource
	Citations             []Citation
}

// BindAnswerSources revalidates the results returned by the shared Search
// function and freezes the exact ordered source set before provider I/O.
func (s *Store) BindAnswerSources(ctx context.Context, answerID string, hits []ChunkHit) ([]AnswerSource, error) {
	if err := validateIdentifier("answer message id", answerID); err != nil {
		return nil, err
	}
	if err := validateAnswerHits(hits); err != nil {
		return nil, err
	}
	bound := make([]AnswerSource, 0, len(hits))
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		scope, err := pendingAskScope(ctx, tx, answerID)
		if err != nil {
			return err
		}
		var existing int
		if err := tx.QueryRowContext(ctx, `
			SELECT count(*) FROM answer_sources WHERE answer_message_id = ?
		`, answerID).Scan(&existing); err != nil {
			return fmt.Errorf("inspect existing answer sources: %w", err)
		}
		if existing != 0 {
			return ErrAnswerNotPending
		}
		for index, hit := range hits {
			var actual ChunkHit
			var title string
			err := tx.QueryRowContext(ctx, `
				SELECT c.id, c.document_id, c.revision_id, c.ordinal, c.content, d.title
				FROM chunks AS c
				JOIN documents AS d ON d.id = c.document_id
				JOIN document_revisions AS r
					ON r.document_id = c.document_id AND r.id = c.revision_id
				WHERE c.id = ? AND d.status = 'active' AND r.is_active = 1
					AND (? IS NULL OR EXISTS (
						SELECT 1 FROM collection_documents AS cd
						WHERE cd.collection_id = ? AND cd.document_id = c.document_id
					))
			`, hit.ChunkID, nullStringValue(scope), nullStringValue(scope)).Scan(
				&actual.ChunkID, &actual.DocumentID, &actual.RevisionID,
				&actual.Ordinal, &actual.Content, &title,
			)
			if errors.Is(err, sql.ErrNoRows) {
				return ErrAnswerSourceChanged
			}
			if err != nil {
				return fmt.Errorf("revalidate answer source: %w", err)
			}
			if actual.ChunkID != hit.ChunkID || actual.DocumentID != hit.DocumentID ||
				actual.RevisionID != hit.RevisionID || actual.Ordinal != hit.Ordinal ||
				actual.Content != hit.Content || !validDisplayText(title, maxDocumentTitleBytes) {
				return ErrAnswerSourceChanged
			}
			digest := sha256.Sum256([]byte(actual.Content))
			source := AnswerSource{
				Position: index + 1, ChunkID: actual.ChunkID,
				DocumentID: actual.DocumentID, RevisionID: actual.RevisionID,
				ChunkOrdinal: actual.Ordinal, ContentHash: hex.EncodeToString(digest[:]),
				DocumentTitle: title,
				Content:       actual.Content,
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO answer_sources(
					answer_message_id, source_position, chunk_id, document_id,
					revision_id, chunk_ordinal, content_hash, document_title
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			`, answerID, source.Position, source.ChunkID, source.DocumentID,
				source.RevisionID, source.ChunkOrdinal, source.ContentHash,
				source.DocumentTitle); err != nil {
				return fmt.Errorf("freeze answer source: %w", err)
			}
			bound = append(bound, source)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return bound, nil
}

// ArmAnswerInvocation moves the persisted recovery deadline to the actual
// provider-call boundary. It races safely with runtime reconciliation: either
// this transaction extends a still-pending row, or reconciliation wins and no
// provider call is allowed to start.
func (s *Store) ArmAnswerInvocation(ctx context.Context, answerID string) (time.Time, error) {
	if err := validateIdentifier("answer message id", answerID); err != nil {
		return time.Time{}, err
	}
	var deadline int64
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		var status string
		var timeoutMillis, createdAt, currentDeadline int64
		err := tx.QueryRowContext(ctx, `
			SELECT m.status, c.timeout_milliseconds, m.created_at, m.reconcile_after
			FROM conversation_messages AS m
			JOIN ollama_config_versions AS c
				ON c.config_version = m.provider_config_version
			WHERE m.id = ? AND m.role = 'assistant'
		`, answerID).Scan(&status, &timeoutMillis, &createdAt, &currentDeadline)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("read answer invocation deadline: %w", err)
		}
		if MessageStatus(status) != MessagePending {
			return ErrAnswerNotPending
		}
		base := max(s.nowMicros(), createdAt)
		deadline, err = addAnswerReconcileDeadline(base, time.Duration(timeoutMillis)*time.Millisecond)
		if err != nil {
			return err
		}
		deadline = max(deadline, currentDeadline)
		result, err := tx.ExecContext(ctx, `
			UPDATE conversation_messages SET reconcile_after = ?
			WHERE id = ? AND role = 'assistant' AND status = 'pending'
		`, deadline, answerID)
		if err != nil {
			return fmt.Errorf("arm answer invocation deadline: %w", err)
		}
		count, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("count armed answer invocation: %w", err)
		}
		if count != 1 {
			return ErrAnswerNotPending
		}
		return nil
	})
	if err != nil {
		return time.Time{}, err
	}
	return time.UnixMicro(deadline).UTC(), nil
}

// CompleteAnswer atomically revalidates the frozen source set, records only
// citations to that set, and publishes the answer. A lifecycle/scope change is
// itself durably terminal and returns ErrAnswerSourceChanged to the caller.
func (s *Store) CompleteAnswer(ctx context.Context, answerID, content string, citationPositions []int) error {
	if err := validateIdentifier("answer message id", answerID); err != nil {
		return err
	}
	if !validAnswerText(content) {
		return fmt.Errorf("sqlite: answer must contain 1 to %d safe UTF-8 bytes", maxAnswerBytes)
	}
	if len(citationPositions) < 1 || len(citationPositions) > maxAnswerCitations {
		return fmt.Errorf("sqlite: answer must contain 1 to %d citations", maxAnswerCitations)
	}
	sourceChanged := false
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		scope, err := pendingAskScope(ctx, tx, answerID)
		if err != nil {
			return err
		}
		valid, sourceCount, err := answerSourcesStillValid(ctx, tx, answerID, scope)
		if err != nil {
			return err
		}
		if !valid || sourceCount == 0 {
			if err := s.setAnswerTerminal(ctx, tx, answerID, MessageFailed,
				SourceChangedText, "SOURCE_CHANGED", "SOURCE_CHANGED"); err != nil {
				return err
			}
			sourceChanged = true
			return nil
		}
		for occurrence, position := range citationPositions {
			if position < 1 || position > sourceCount {
				return ErrInvalidCitation
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO answer_citations(answer_message_id, occurrence, source_position)
				VALUES (?, ?, ?)
			`, answerID, occurrence+1, position); err != nil {
				return fmt.Errorf("record answer citation: %w", err)
			}
		}
		return s.setAnswerTerminal(ctx, tx, answerID, MessageCompleted, content, "", "")
	})
	if err != nil {
		return err
	}
	if sourceChanged {
		return ErrAnswerSourceChanged
	}
	return nil
}

func (s *Store) RefuseAnswer(ctx context.Context, answerID, content, limitationCode string) error {
	if err := validateIdentifier("answer message id", answerID); err != nil {
		return err
	}
	if !validAnswerText(content) || !validRAGSafeCode(limitationCode) {
		return errors.New("sqlite: invalid controlled refusal")
	}
	return s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := pendingAskScope(ctx, tx, answerID); err != nil {
			return err
		}
		return s.setAnswerTerminal(ctx, tx, answerID, MessageRefused, content, limitationCode, "")
	})
}

func (s *Store) FailAnswer(ctx context.Context, answerID, content, limitationCode, errorCode string) error {
	if err := validateIdentifier("answer message id", answerID); err != nil {
		return err
	}
	if !validAnswerText(content) || !validRAGSafeCode(limitationCode) || !validRAGSafeCode(errorCode) {
		return errors.New("sqlite: invalid controlled answer failure")
	}
	return s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := pendingAskScope(ctx, tx, answerID); err != nil {
			return err
		}
		return s.setAnswerTerminal(ctx, tx, answerID, MessageFailed, content, limitationCode, errorCode)
	})
}

// ReconcileAllPendingAnswers is the startup-only recovery pass, called after
// acquiring the Vault lock and before ordinary writes are served. Every row
// belongs to the previous process and may have crossed the network boundary.
func (s *Store) ReconcileAllPendingAnswers(ctx context.Context) (int64, error) {
	if ctx == nil {
		return 0, errors.New("sqlite: nil context")
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE conversation_messages
		SET status = 'failed', content = ?, limitation_code = 'OUTCOME_UNCERTAIN',
			error_code = 'OUTCOME_UNCERTAIN', completed_at = max(?, created_at)
		WHERE role = 'assistant' AND status = 'pending'
	`, OutcomeUncertainText, s.nowMicros())
	if err != nil {
		return 0, fmt.Errorf("sqlite: reconcile pending answers: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("sqlite: count reconciled pending answers: %w", err)
	}
	return count, nil
}

// ReconcileExpiredPendingAnswers is the bounded same-process maintenance pass.
// It never touches a provider call whose persisted deadline is still in the
// future, and it never replays provider I/O.
func (s *Store) ReconcileExpiredPendingAnswers(ctx context.Context, limit int) (int64, error) {
	if ctx == nil {
		return 0, errors.New("sqlite: nil context")
	}
	if limit < 1 || limit > maxPendingReconcileBatch {
		return 0, fmt.Errorf("sqlite: pending answer reconciliation limit must be between 1 and %d", maxPendingReconcileBatch)
	}
	now := s.nowMicros()
	result, err := s.db.ExecContext(ctx, `
		UPDATE conversation_messages
		SET status = 'failed', content = ?, limitation_code = 'OUTCOME_UNCERTAIN',
			error_code = 'OUTCOME_UNCERTAIN', completed_at = max(?, created_at)
		WHERE id IN (
			SELECT id FROM conversation_messages
			WHERE role = 'assistant' AND status = 'pending' AND reconcile_after <= ?
			ORDER BY reconcile_after, created_at, id
			LIMIT ?
		)
		AND role = 'assistant' AND status = 'pending' AND reconcile_after <= ?
	`, OutcomeUncertainText, now, now, limit, now)
	if err != nil {
		return 0, fmt.Errorf("sqlite: reconcile expired pending answers: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("sqlite: count reconciled expired answers: %w", err)
	}
	return count, nil
}

func (s *Store) GetAnswer(ctx context.Context, answerID string) (Answer, error) {
	if err := validateIdentifier("answer message id", answerID); err != nil {
		return Answer{}, err
	}
	if ctx == nil {
		return Answer{}, errors.New("sqlite: nil context")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Answer{}, fmt.Errorf("sqlite: begin answer read: %w", err)
	}
	defer tx.Rollback()
	answer, err := readAnswer(ctx, tx, answerID)
	if err != nil {
		return Answer{}, err
	}
	if err := tx.Commit(); err != nil {
		return Answer{}, fmt.Errorf("sqlite: finish answer read: %w", err)
	}
	return answer, nil
}

func readAnswer(ctx context.Context, tx *sql.Tx, answerID string) (Answer, error) {
	var answer Answer
	var status string
	var scope, limitation, errorCode sql.NullString
	var createdAt int64
	var completedAt sql.NullInt64
	var reconcileAfter int64
	err := tx.QueryRowContext(ctx, `
		SELECT assistant.id, assistant.conversation_id, c.revision, user.content,
			assistant.status, assistant.content, assistant.provider_config_version,
			a.scope_collection_id, assistant.limitation_code, assistant.error_code,
			assistant.created_at, assistant.completed_at, assistant.reconcile_after
		FROM conversation_messages AS assistant
		JOIN ask_requests AS a ON a.answer_message_id = assistant.id
		JOIN conversation_messages AS user ON user.id = a.user_message_id
		JOIN conversations AS c ON c.id = assistant.conversation_id
		WHERE assistant.id = ? AND assistant.role = 'assistant'
	`, answerID).Scan(
		&answer.ID, &answer.ConversationID, &answer.ConversationRevision,
		&answer.Question, &status, &answer.Content, &answer.ProviderConfigVersion,
		&scope, &limitation, &errorCode, &createdAt, &completedAt, &reconcileAfter,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Answer{}, ErrNotFound
	}
	if err != nil {
		return Answer{}, fmt.Errorf("sqlite: read answer: %w", err)
	}
	answer.Status = MessageStatus(status)
	answer.LimitationCode = limitation.String
	answer.ErrorCode = errorCode.String
	answer.CreatedAt = time.UnixMicro(createdAt).UTC()
	answer.ReconcileAfter = time.UnixMicro(reconcileAfter).UTC()
	if scope.Valid {
		value := scope.String
		answer.ScopeCollectionID = &value
	}
	if completedAt.Valid {
		value := time.UnixMicro(completedAt.Int64).UTC()
		answer.CompletedAt = &value
	}

	answer.Sources, answer.Citations, err = readAnswerEvidence(ctx, tx, answerID)
	if err != nil {
		return Answer{}, err
	}
	return answer, nil
}

func readAnswerEvidence(ctx context.Context, tx *sql.Tx, answerID string) ([]AnswerSource, []Citation, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT s.source_position, s.chunk_id, s.document_id, s.revision_id,
			s.chunk_ordinal, s.content_hash, s.document_title, c.content
		FROM answer_sources AS s
		JOIN chunks AS c ON c.id = s.chunk_id
		WHERE s.answer_message_id = ? ORDER BY s.source_position
	`, answerID)
	if err != nil {
		return nil, nil, fmt.Errorf("sqlite: read answer sources: %w", err)
	}
	sources := make([]AnswerSource, 0)
	for rows.Next() {
		var source AnswerSource
		if err := rows.Scan(&source.Position, &source.ChunkID, &source.DocumentID,
			&source.RevisionID, &source.ChunkOrdinal, &source.ContentHash,
			&source.DocumentTitle, &source.Content); err != nil {
			_ = rows.Close()
			return nil, nil, fmt.Errorf("sqlite: scan answer source: %w", err)
		}
		digest := sha256.Sum256([]byte(source.Content))
		if hex.EncodeToString(digest[:]) != source.ContentHash {
			_ = rows.Close()
			return nil, nil, ErrAnswerSourceChanged
		}
		sources = append(sources, source)
	}
	if err := rows.Close(); err != nil {
		return nil, nil, fmt.Errorf("sqlite: close answer sources: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("sqlite: iterate answer sources: %w", err)
	}

	rows, err = tx.QueryContext(ctx, `
		SELECT occurrence, source_position FROM answer_citations
		WHERE answer_message_id = ? ORDER BY occurrence
	`, answerID)
	if err != nil {
		return nil, nil, fmt.Errorf("sqlite: read answer citations: %w", err)
	}
	citations := make([]Citation, 0)
	for rows.Next() {
		var citation Citation
		if err := rows.Scan(&citation.Occurrence, &citation.Position); err != nil {
			_ = rows.Close()
			return nil, nil, fmt.Errorf("sqlite: scan answer citation: %w", err)
		}
		citations = append(citations, citation)
	}
	if err := rows.Close(); err != nil {
		return nil, nil, fmt.Errorf("sqlite: close answer citations: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("sqlite: iterate answer citations: %w", err)
	}
	return sources, citations, nil
}

func pendingAskScope(ctx context.Context, tx *sql.Tx, answerID string) (sql.NullString, error) {
	var status string
	var scope sql.NullString
	err := tx.QueryRowContext(ctx, `
		SELECT m.status, a.scope_collection_id
		FROM conversation_messages AS m
		JOIN ask_requests AS a ON a.answer_message_id = m.id
		WHERE m.id = ? AND m.role = 'assistant'
	`, answerID).Scan(&status, &scope)
	if errors.Is(err, sql.ErrNoRows) {
		return sql.NullString{}, ErrNotFound
	}
	if err != nil {
		return sql.NullString{}, fmt.Errorf("read pending answer: %w", err)
	}
	if MessageStatus(status) != MessagePending {
		return sql.NullString{}, ErrAnswerNotPending
	}
	return scope, nil
}

func answerSourcesStillValid(ctx context.Context, tx *sql.Tx, answerID string, scope sql.NullString) (bool, int, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT s.source_position, s.chunk_id, s.document_id, s.revision_id,
			s.chunk_ordinal, s.content_hash, s.document_title,
			c.id, c.document_id, c.revision_id, c.ordinal, c.content, d.title,
			d.status, r.is_active,
			CASE WHEN ? IS NULL THEN 1 ELSE EXISTS (
				SELECT 1 FROM collection_documents AS cd
				WHERE cd.collection_id = ? AND cd.document_id = c.document_id
			) END
		FROM answer_sources AS s
		LEFT JOIN chunks AS c ON c.id = s.chunk_id
		LEFT JOIN documents AS d ON d.id = c.document_id
		LEFT JOIN document_revisions AS r
			ON r.document_id = c.document_id AND r.id = c.revision_id
		WHERE s.answer_message_id = ?
		ORDER BY s.source_position
	`, nullStringValue(scope), nullStringValue(scope), answerID)
	if err != nil {
		return false, 0, fmt.Errorf("read frozen answer sources: %w", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var source AnswerSource
		var chunkID, documentID, revisionID, content, title, status sql.NullString
		var ordinal, active, inScope sql.NullInt64
		if err := rows.Scan(&source.Position, &source.ChunkID, &source.DocumentID,
			&source.RevisionID, &source.ChunkOrdinal, &source.ContentHash,
			&source.DocumentTitle, &chunkID, &documentID, &revisionID,
			&ordinal, &content, &title, &status, &active, &inScope); err != nil {
			return false, 0, fmt.Errorf("scan frozen answer source: %w", err)
		}
		count++
		if source.Position != count || !chunkID.Valid || chunkID.String != source.ChunkID ||
			!documentID.Valid || documentID.String != source.DocumentID ||
			!revisionID.Valid || revisionID.String != source.RevisionID ||
			!ordinal.Valid || int(ordinal.Int64) != source.ChunkOrdinal ||
			!content.Valid || !title.Valid || title.String != source.DocumentTitle ||
			!status.Valid || status.String != "active" || !active.Valid || active.Int64 != 1 ||
			!inScope.Valid || inScope.Int64 != 1 {
			return false, count, nil
		}
		digest := sha256.Sum256([]byte(content.String))
		if hex.EncodeToString(digest[:]) != source.ContentHash {
			return false, count, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, count, fmt.Errorf("iterate frozen answer sources: %w", err)
	}
	return true, count, nil
}

func (s *Store) setAnswerTerminal(ctx context.Context, tx *sql.Tx, answerID string, status MessageStatus, content, limitation, errorCode string) error {
	nowMicros := s.nowMicros()
	result, err := tx.ExecContext(ctx, `
		UPDATE conversation_messages
		SET status = ?, content = ?, limitation_code = NULLIF(?, ''),
			error_code = NULLIF(?, ''), completed_at = max(?, created_at)
		WHERE id = ? AND role = 'assistant' AND status = 'pending'
	`, string(status), content, limitation, errorCode, nowMicros, answerID)
	if err != nil {
		return fmt.Errorf("publish answer terminal state: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count published answer state: %w", err)
	}
	if count != 1 {
		return ErrAnswerNotPending
	}
	return nil
}

func validateOllamaConfig(params SaveOllamaConfigParams) error {
	if params.ExpectedVersion < 0 {
		return errors.New("sqlite: expected Ollama configuration version must not be negative")
	}
	if !validOpaqueText(params.Endpoint, maxOllamaEndpointBytes) {
		return fmt.Errorf("sqlite: Ollama endpoint must contain 1 to %d safe UTF-8 bytes", maxOllamaEndpointBytes)
	}
	if !validOpaqueText(params.Model, maxOllamaModelBytes) {
		return fmt.Errorf("sqlite: Ollama model must contain 1 to %d safe UTF-8 bytes", maxOllamaModelBytes)
	}
	if params.Timeout < time.Millisecond || params.Timeout > 10*time.Minute || params.Timeout%time.Millisecond != 0 {
		return errors.New("sqlite: Ollama timeout must be whole milliseconds between 1ms and 10m")
	}
	return nil
}

func addAnswerReconcileDeadline(createdAt int64, providerTimeout time.Duration) (int64, error) {
	delta := providerTimeout.Microseconds() + answerReconcileGrace.Microseconds()
	const maxInt64 = int64(^uint64(0) >> 1)
	if createdAt < 0 || delta <= 0 || createdAt > maxInt64-delta {
		return 0, errors.New("sqlite: answer reconciliation deadline overflow")
	}
	return createdAt + delta, nil
}

func validateHistoryPage(after *HistoryCursor, limit int) error {
	if limit < 1 || limit > maxHistoryPageSize {
		return fmt.Errorf("sqlite: history page limit must be between 1 and %d", maxHistoryPageSize)
	}
	if after == nil {
		return nil
	}
	if err := validateIdentifier("history cursor id", after.ID); err != nil {
		return err
	}
	if after.CreatedAt.Before(time.Unix(0, 0)) || after.CreatedAt.Nanosecond()%1_000 != 0 {
		return errors.New("sqlite: history cursor time must be a non-negative exact microsecond")
	}
	return nil
}

func validateBeginAsk(params BeginAskParams) error {
	for field, value := range map[string]string{
		"conversation id":   params.ConversationID,
		"user message id":   params.UserMessageID,
		"answer message id": params.AnswerMessageID,
	} {
		if err := validateIdentifier(field, value); err != nil {
			return err
		}
	}
	if params.UserMessageID == params.AnswerMessageID {
		return errors.New("sqlite: user and answer message identifiers must differ")
	}
	if params.ExpectedRevision < 0 {
		return errors.New("sqlite: expected conversation revision must not be negative")
	}
	if !validOpaqueText(params.IdempotencyKey, maxIdempotencyKeyBytes) {
		return fmt.Errorf("sqlite: Ask idempotency key must contain 1 to %d safe UTF-8 bytes", maxIdempotencyKeyBytes)
	}
	if !validLowerHex(params.RequestHash, sha256.Size*2) {
		return errors.New("sqlite: Ask request hash must be lowercase SHA-256")
	}
	if !validQuestion(params.Question) {
		return fmt.Errorf("sqlite: Ask question must contain 1 to %d safe UTF-8 bytes", maxAskQuestionBytes)
	}
	if params.ScopeCollectionID != nil {
		if err := validateIdentifier("scope collection id", *params.ScopeCollectionID); err != nil {
			return err
		}
	}
	return nil
}

func validateAnswerHits(hits []ChunkHit) error {
	if len(hits) < 1 || len(hits) > maxAnswerSources {
		return fmt.Errorf("sqlite: answer source count must be between 1 and %d", maxAnswerSources)
	}
	seen := make(map[string]struct{}, len(hits))
	total := 0
	for _, hit := range hits {
		for field, value := range map[string]string{
			"chunk id": hit.ChunkID, "document id": hit.DocumentID, "revision id": hit.RevisionID,
		} {
			if err := validateIdentifier(field, value); err != nil {
				return err
			}
		}
		if _, exists := seen[hit.ChunkID]; exists {
			return errors.New("sqlite: duplicate answer source chunk")
		}
		seen[hit.ChunkID] = struct{}{}
		if hit.Ordinal < 0 || !utf8.ValidString(hit.Content) || strings.TrimSpace(hit.Content) == "" {
			return errors.New("sqlite: invalid answer source content")
		}
		total += len(hit.Content)
		if total > maxAnswerSourceBytes {
			return fmt.Errorf("sqlite: answer source content exceeds %d bytes", maxAnswerSourceBytes)
		}
	}
	return nil
}

func validQuestion(value string) bool {
	if !utf8.ValidString(value) || len(value) == 0 || len(value) > maxAskQuestionBytes || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if character == 0 || (unicode.IsControl(character) && character != '\t' && character != '\n') {
			return false
		}
	}
	return true
}

func validAnswerText(value string) bool {
	if !utf8.ValidString(value) || len(value) == 0 || len(value) > maxAnswerBytes || strings.TrimSpace(value) == "" {
		return false
	}
	for _, character := range value {
		if character == 0 || (unicode.IsControl(character) && character != '\t' && character != '\n') {
			return false
		}
	}
	return true
}

func validRAGSafeCode(value string) bool {
	if len(value) < 1 || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}

func nullStringValue(value sql.NullString) any {
	if value.Valid {
		return value.String
	}
	return nil
}
