package app

import (
	"errors"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/mt-hub8/MindWeaver/v2/internal/ollama"
	"github.com/mt-hub8/MindWeaver/v2/internal/rag"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/internal/transport"
)

type ollamaConfigView struct {
	Version             int64  `json:"version"`
	Endpoint            string `json:"endpoint"`
	Model               string `json:"model"`
	TimeoutMilliseconds int64  `json:"timeoutMilliseconds"`
	Active              bool   `json:"active"`
	CreatedAt           string `json:"createdAt"`
}

type conversationView struct {
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	Revision        int64   `json:"revision"`
	PendingAnswer   bool    `json:"pendingAnswer"`
	PendingAnswerID *string `json:"pendingAnswerId"`
	CreatedAt       string  `json:"createdAt"`
	UpdatedAt       string  `json:"updatedAt"`
}

type answerSourceView struct {
	Position      int    `json:"position"`
	ChunkID       string `json:"chunkId"`
	DocumentID    string `json:"documentId"`
	RevisionID    string `json:"revisionId"`
	ChunkOrdinal  int    `json:"chunkOrdinal"`
	ContentHash   string `json:"contentHash"`
	DocumentTitle string `json:"documentTitle"`
	Content       string `json:"content"`
}

type citationView struct {
	Occurrence int `json:"occurrence"`
	Position   int `json:"position"`
}

type answerView struct {
	ID                    string             `json:"id"`
	ConversationID        string             `json:"conversationId"`
	ConversationRevision  int64              `json:"conversationRevision"`
	Question              string             `json:"question"`
	Status                string             `json:"status"`
	Content               string             `json:"content"`
	ProviderConfigVersion int64              `json:"providerConfigVersion"`
	ScopeCollectionID     *string            `json:"scopeCollectionId"`
	LimitationCode        string             `json:"limitationCode"`
	ErrorCode             string             `json:"errorCode"`
	CreatedAt             string             `json:"createdAt"`
	CompletedAt           *string            `json:"completedAt"`
	ReconcileAfter        string             `json:"reconcileAfter"`
	Sources               []answerSourceView `json:"sources"`
	Citations             []citationView     `json:"citations"`
}

type messageView struct {
	ID                    string             `json:"id"`
	ConversationID        string             `json:"conversationId"`
	Ordinal               int                `json:"ordinal"`
	Role                  string             `json:"role"`
	Status                string             `json:"status"`
	Content               string             `json:"content"`
	ProviderConfigVersion int64              `json:"providerConfigVersion"`
	ScopeCollectionID     *string            `json:"scopeCollectionId"`
	LimitationCode        string             `json:"limitationCode"`
	ErrorCode             string             `json:"errorCode"`
	CreatedAt             string             `json:"createdAt"`
	CompletedAt           *string            `json:"completedAt"`
	ReconcileAfter        *string            `json:"reconcileAfter"`
	Sources               []answerSourceView `json:"sources"`
	Citations             []citationView     `json:"citations"`
}

func (api *API) ollamaConfiguration(response http.ResponseWriter, request *http.Request) {
	if !noUnexpectedQuery(request) {
		api.problem(response, request, transport.CodeInvalidArgument, "模型配置请求不能包含查询参数。")
		return
	}
	config, err := api.rag.service.GetActiveOllamaConfig(request.Context())
	if errors.Is(err, store.ErrOllamaNotConfigured) {
		writeJSON(response, http.StatusOK, struct {
			Configured bool              `json:"configured"`
			Config     *ollamaConfigView `json:"config"`
		}{Configured: false, Config: nil})
		return
	}
	if err != nil {
		code, detail := classifyError(err)
		api.problem(response, request, code, detail)
		return
	}
	view := viewOllamaConfig(config)
	writeJSON(response, http.StatusOK, struct {
		Configured bool              `json:"configured"`
		Config     *ollamaConfigView `json:"config"`
	}{Configured: true, Config: &view})
}

func (api *API) configureOllama(response http.ResponseWriter, request *http.Request) {
	if !noUnexpectedQuery(request) {
		api.problem(response, request, transport.CodeInvalidArgument, "模型配置请求不能包含查询参数。")
		return
	}
	var body struct {
		ExpectedVersion     *int64 `json:"expectedVersion"`
		Endpoint            string `json:"endpoint"`
		Model               string `json:"model"`
		TimeoutMilliseconds *int64 `json:"timeoutMilliseconds"`
	}
	if err := decodeStrictJSON(request, &body); err != nil || body.ExpectedVersion == nil || *body.ExpectedVersion < 0 || !validOllamaTimeout(body.TimeoutMilliseconds) {
		api.problem(response, request, transport.CodeInvalidArgument, "模型配置必须包含有效的 endpoint、model、timeoutMilliseconds 与 expectedVersion。")
		return
	}
	config, err := api.rag.service.ConfigureOllama(request.Context(), *body.ExpectedVersion, ollama.Options{
		BaseURL: body.Endpoint, Model: body.Model,
		Timeout: time.Duration(*body.TimeoutMilliseconds) * time.Millisecond,
	})
	if errors.Is(err, store.ErrProviderConfigRevision) && *body.ExpectedVersion < int64(^uint64(0)>>1) {
		// PUT is retry-safe when the first response was lost: the immediately
		// following active version must exactly equal the requested state.
		current, readErr := api.rag.service.GetActiveOllamaConfig(request.Context())
		if readErr == nil && current.Version == *body.ExpectedVersion+1 &&
			current.Endpoint == body.Endpoint && current.Model == body.Model &&
			current.Timeout == time.Duration(*body.TimeoutMilliseconds)*time.Millisecond {
			config, err = current, nil
		}
	}
	if err != nil {
		code, detail := classifyError(err)
		api.problem(response, request, code, detail)
		return
	}
	writeJSON(response, http.StatusOK, struct {
		Config ollamaConfigView `json:"config"`
	}{Config: viewOllamaConfig(config)})
}

func (api *API) probeOllama(response http.ResponseWriter, request *http.Request) {
	if !noUnexpectedQuery(request) {
		api.problem(response, request, transport.CodeInvalidArgument, "模型探测请求不能包含查询参数。")
		return
	}
	var body struct {
		Endpoint            string `json:"endpoint"`
		Model               string `json:"model"`
		TimeoutMilliseconds *int64 `json:"timeoutMilliseconds"`
	}
	if err := decodeStrictJSON(request, &body); err != nil || !validOllamaTimeout(body.TimeoutMilliseconds) {
		api.problem(response, request, transport.CodeInvalidArgument, "模型探测必须包含有效的 endpoint、model 与 timeoutMilliseconds。")
		return
	}
	models, err := api.rag.service.ProbeOllama(request.Context(), ollama.Options{
		BaseURL: body.Endpoint, Model: body.Model,
		Timeout: time.Duration(*body.TimeoutMilliseconds) * time.Millisecond,
	})
	if err != nil {
		code, detail := classifyError(err)
		api.problem(response, request, code, detail)
		return
	}
	if models == nil {
		models = []string{}
	}
	writeJSON(response, http.StatusOK, struct {
		Available bool     `json:"available"`
		Models    []string `json:"models"`
	}{Available: true, Models: models})
}

func (api *API) createConversation(response http.ResponseWriter, request *http.Request) {
	if !noUnexpectedQuery(request) {
		api.problem(response, request, transport.CodeInvalidArgument, "创建会话不能包含查询参数。")
		return
	}
	key, ok := singleHeader(request.Header, "Idempotency-Key")
	if !ok || !validOpaque(key, 256) {
		api.problem(response, request, transport.CodeInvalidArgument, "Idempotency-Key 必须是 1 到 256 字节的非空值。")
		return
	}
	var body struct {
		Title string `json:"title"`
	}
	if err := decodeStrictJSON(request, &body); err != nil || !validDisplay(body.Title, maxHeaderText) {
		api.problem(response, request, transport.CodeInvalidArgument, "会话标题必须是 1 到 1024 字节的有效文本。")
		return
	}
	conversation, created, err := api.rag.service.CreateConversationIdempotent(request.Context(), key, body.Title)
	if err != nil {
		code, detail := classifyError(err)
		api.problem(response, request, code, detail)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(response, status, struct {
		Conversation conversationView `json:"conversation"`
		Created      bool             `json:"created"`
	}{Conversation: viewConversation(conversation), Created: created})
}

func (api *API) conversations(response http.ResponseWriter, request *http.Request) {
	if !noUnexpectedQuery(request, "limit", "cursor") {
		api.problem(response, request, transport.CodeInvalidArgument, "会话目录包含未知查询参数。")
		return
	}
	limit, err := queryLimit(request, 50)
	if err != nil || limit > 50 {
		api.problem(response, request, transport.CodeInvalidArgument, "limit 必须是 1 到 50。")
		return
	}
	var after *store.HistoryCursor
	if raw, exists := optionalQuery(request, "cursor"); exists {
		stamp, id, cursorErr := decodeAPICursor(raw, conversationCursorKind, "")
		if cursorErr != nil {
			api.problem(response, request, transport.CodeInvalidArgument, "会话 cursor 无效或不属于此目录。")
			return
		}
		after = &store.HistoryCursor{CreatedAt: stamp, ID: id}
	}
	page, err := api.rag.service.ListConversations(request.Context(), after, limit)
	if err != nil {
		code, detail := classifyError(err)
		api.problem(response, request, code, detail)
		return
	}
	items := make([]conversationView, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, viewConversation(item))
	}
	next := ""
	if page.NextCursor != nil {
		next, err = encodeAPICursor(conversationCursorKind, "", page.NextCursor.CreatedAt, page.NextCursor.ID)
		if err != nil {
			api.problem(response, request, transport.CodeInternal, "会话目录游标编码失败。")
			return
		}
	}
	writeJSON(response, http.StatusOK, struct {
		Conversations []conversationView `json:"conversations"`
		NextCursor    string             `json:"nextCursor,omitempty"`
	}{Conversations: items, NextCursor: next})
}

func (api *API) deleteConversation(response http.ResponseWriter, request *http.Request) {
	if !noUnexpectedQuery(request) {
		api.problem(response, request, transport.CodeInvalidArgument, "删除会话不能包含查询参数。")
		return
	}
	var body struct {
		ConversationID   string `json:"conversationId"`
		ExpectedRevision *int64 `json:"expectedRevision"`
	}
	if err := decodeStrictJSON(request, &body); err != nil || !validIdentifier(body.ConversationID) || body.ExpectedRevision == nil || *body.ExpectedRevision < 0 {
		api.problem(response, request, transport.CodeInvalidArgument, "删除会话必须包含有效且唯一的 conversationId 与 expectedRevision。")
		return
	}
	deleted, err := api.rag.service.DeleteConversation(request.Context(), body.ConversationID, *body.ExpectedRevision)
	if err != nil {
		code, detail := classifyError(err)
		api.problem(response, request, code, detail)
		return
	}
	writeJSON(response, http.StatusOK, struct {
		ConversationID string `json:"conversationId"`
		Deleted        bool   `json:"deleted"`
	}{ConversationID: body.ConversationID, Deleted: deleted})
}

func (api *API) conversationMessages(response http.ResponseWriter, request *http.Request) {
	if !noUnexpectedQuery(request, "conversation_id", "limit", "cursor") {
		api.problem(response, request, transport.CodeInvalidArgument, "消息历史包含未知查询参数。")
		return
	}
	conversationID, ok := oneQuery(request, "conversation_id")
	if !ok || !validIdentifier(conversationID) {
		api.problem(response, request, transport.CodeInvalidArgument, "conversation_id 必须是唯一且有效的标识符。")
		return
	}
	limit, err := queryLimit(request, 50)
	if err != nil || limit > 50 {
		api.problem(response, request, transport.CodeInvalidArgument, "limit 必须是 1 到 50。")
		return
	}
	var after *store.HistoryCursor
	if raw, exists := optionalQuery(request, "cursor"); exists {
		stamp, id, cursorErr := decodeAPICursor(raw, conversationMessageCursorKind, conversationID)
		if cursorErr != nil {
			api.problem(response, request, transport.CodeInvalidArgument, "消息 cursor 无效或不属于此会话。")
			return
		}
		after = &store.HistoryCursor{CreatedAt: stamp, ID: id}
	}
	page, err := api.rag.service.ListConversationMessages(request.Context(), conversationID, after, limit)
	if err != nil {
		code, detail := classifyError(err)
		api.problem(response, request, code, detail)
		return
	}
	items := make([]messageView, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, viewMessage(item))
	}
	next := ""
	if page.NextCursor != nil {
		next, err = encodeAPICursor(conversationMessageCursorKind, conversationID, page.NextCursor.CreatedAt, page.NextCursor.ID)
		if err != nil {
			api.problem(response, request, transport.CodeInternal, "消息历史游标编码失败。")
			return
		}
	}
	writeJSON(response, http.StatusOK, struct {
		Messages   []messageView `json:"messages"`
		NextCursor string        `json:"nextCursor,omitempty"`
	}{Messages: items, NextCursor: next})
}

func (api *API) ask(response http.ResponseWriter, request *http.Request) {
	if !noUnexpectedQuery(request) {
		api.problem(response, request, transport.CodeInvalidArgument, "Ask 请求不能包含查询参数。")
		return
	}
	key, ok := singleHeader(request.Header, "Idempotency-Key")
	if !ok || !validOpaque(key, 256) {
		api.problem(response, request, transport.CodeInvalidArgument, "Idempotency-Key 必须是 1 到 256 字节的非空值。")
		return
	}
	var body struct {
		ConversationID    string  `json:"conversationId"`
		ExpectedRevision  *int64  `json:"expectedRevision"`
		Question          string  `json:"question"`
		ScopeCollectionID *string `json:"scopeCollectionId"`
	}
	if err := decodeStrictJSON(request, &body); err != nil || !validIdentifier(body.ConversationID) || body.ExpectedRevision == nil || *body.ExpectedRevision < 0 || !validDisplay(body.Question, rag.MaxQuestionBytes) || (body.ScopeCollectionID != nil && !validIdentifier(*body.ScopeCollectionID)) {
		api.problem(response, request, transport.CodeInvalidArgument, "Ask 必须包含有效且唯一的 conversationId、expectedRevision、不超过 1024 UTF-8 字节的 question 与可选 scopeCollectionId。")
		return
	}
	answer, err := api.rag.Ask(request.Context(), rag.AskRequest{
		ConversationID: body.ConversationID, ExpectedRevision: *body.ExpectedRevision,
		IdempotencyKey: key, Question: body.Question, ScopeCollectionID: cloneOptionalString(body.ScopeCollectionID),
	})
	if validTerminalAnswer(answer) {
		writeJSON(response, http.StatusOK, struct {
			Answer answerView `json:"answer"`
		}{Answer: viewAnswer(answer)})
		return
	}
	if validPendingAnswer(answer) {
		response.Header().Set("Retry-After", "1")
		writeJSON(response, http.StatusAccepted, struct {
			Answer answerView `json:"answer"`
		}{Answer: viewAnswer(answer)})
		return
	}
	if err == nil {
		err = errors.New("app: Ask returned no readable durable answer")
	}
	code, detail := classifyError(err)
	api.problem(response, request, code, detail)
}

func (api *API) answer(response http.ResponseWriter, request *http.Request) {
	if !noUnexpectedQuery(request, "id") {
		api.problem(response, request, transport.CodeInvalidArgument, "回答请求包含未知查询参数。")
		return
	}
	id, ok := oneQuery(request, "id")
	if !ok || !validIdentifier(id) {
		api.problem(response, request, transport.CodeInvalidArgument, "id 必须是唯一且有效的回答标识符。")
		return
	}
	answer, err := api.rag.service.GetAnswer(request.Context(), id)
	if err != nil {
		code, detail := classifyError(err)
		api.problem(response, request, code, detail)
		return
	}
	status := http.StatusOK
	switch {
	case validPendingAnswer(answer):
		status = http.StatusAccepted
		response.Header().Set("Retry-After", "1")
	case validTerminalAnswer(answer):
	default:
		api.problem(response, request, transport.CodeInternal, "持久回答状态无效；请在诊断面板检查本地数据库。")
		return
	}
	writeJSON(response, status, struct {
		Answer answerView `json:"answer"`
	}{Answer: viewAnswer(answer)})
}

func validOllamaTimeout(value *int64) bool {
	return value != nil && *value >= 1 && *value <= rag.MaxOllamaTimeout.Milliseconds()
}

func viewOllamaConfig(config store.OllamaConfig) ollamaConfigView {
	return ollamaConfigView{
		Version: config.Version, Endpoint: config.Endpoint, Model: config.Model,
		TimeoutMilliseconds: config.Timeout.Milliseconds(), Active: config.Active,
		CreatedAt: formatTime(config.CreatedAt),
	}
}

func viewConversation(conversation store.Conversation) conversationView {
	return conversationView{
		ID: conversation.ID, Title: conversation.Title, Revision: conversation.Revision,
		PendingAnswer: conversation.PendingAnswer, PendingAnswerID: cloneOptionalString(conversation.PendingAnswerID),
		CreatedAt: formatTime(conversation.CreatedAt), UpdatedAt: formatTime(conversation.UpdatedAt),
	}
}

func viewAnswer(answer store.Answer) answerView {
	completed := optionalTime(answer.CompletedAt)
	reconcileAfter := ""
	if !answer.ReconcileAfter.IsZero() {
		reconcileAfter = formatTime(answer.ReconcileAfter)
	}
	return answerView{
		ID: answer.ID, ConversationID: answer.ConversationID,
		ConversationRevision: answer.ConversationRevision, Question: answer.Question,
		Status: string(answer.Status), Content: answer.Content,
		ProviderConfigVersion: answer.ProviderConfigVersion,
		ScopeCollectionID:     cloneOptionalString(answer.ScopeCollectionID),
		LimitationCode:        answer.LimitationCode, ErrorCode: answer.ErrorCode,
		CreatedAt: formatTime(answer.CreatedAt), CompletedAt: completed,
		ReconcileAfter: reconcileAfter, Sources: viewSources(answer.Sources),
		Citations: viewCitations(answer.Citations),
	}
}

func viewMessage(message store.ConversationMessage) messageView {
	return messageView{
		ID: message.ID, ConversationID: message.ConversationID, Ordinal: message.Ordinal,
		Role: message.Role, Status: string(message.Status), Content: message.Content,
		ProviderConfigVersion: message.ProviderConfigVersion,
		ScopeCollectionID:     cloneOptionalString(message.ScopeCollectionID),
		LimitationCode:        message.LimitationCode, ErrorCode: message.ErrorCode,
		CreatedAt: formatTime(message.CreatedAt), CompletedAt: optionalTime(message.CompletedAt),
		ReconcileAfter: optionalTime(message.ReconcileAfter), Sources: viewSources(message.Sources),
		Citations: viewCitations(message.Citations),
	}
}

func viewSources(sources []store.AnswerSource) []answerSourceView {
	views := make([]answerSourceView, 0, len(sources))
	for _, source := range sources {
		views = append(views, answerSourceView{
			Position: source.Position, ChunkID: source.ChunkID, DocumentID: source.DocumentID,
			RevisionID: source.RevisionID, ChunkOrdinal: source.ChunkOrdinal,
			ContentHash: source.ContentHash, DocumentTitle: source.DocumentTitle,
			Content: source.Content,
		})
	}
	return views
}

func viewCitations(citations []store.Citation) []citationView {
	views := make([]citationView, 0, len(citations))
	for _, citation := range citations {
		views = append(views, citationView{Occurrence: citation.Occurrence, Position: citation.Position})
	}
	return views
}

func optionalTime(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := formatTime(*value)
	return &formatted
}

func cloneOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func validPendingAnswer(answer store.Answer) bool {
	return validAnswerIdentity(answer) && answer.Status == store.MessagePending &&
		answer.Content == "" && answer.LimitationCode == "" && answer.ErrorCode == "" &&
		answer.CompletedAt == nil && len(answer.Citations) == 0 && validAnswerSources(answer.Sources)
}

func validTerminalAnswer(answer store.Answer) bool {
	if !validAnswerIdentity(answer) || answer.CompletedAt == nil || answer.CompletedAt.Before(answer.CreatedAt) ||
		!validDisplay(answer.Content, 256<<10) || !validAnswerSources(answer.Sources) || !validAnswerCitations(answer.Citations, len(answer.Sources)) {
		return false
	}
	switch answer.Status {
	case store.MessageCompleted:
		return answer.LimitationCode == "" && answer.ErrorCode == "" && len(answer.Sources) > 0 && len(answer.Citations) > 0
	case store.MessageRefused:
		return validOpaque(answer.LimitationCode, 64) && answer.ErrorCode == "" && len(answer.Citations) == 0
	case store.MessageFailed:
		return validOpaque(answer.LimitationCode, 64) && validOpaque(answer.ErrorCode, 64) && len(answer.Citations) == 0
	default:
		return false
	}
}

func validAnswerIdentity(answer store.Answer) bool {
	return validIdentifier(answer.ID) && validIdentifier(answer.ConversationID) &&
		answer.ConversationRevision >= 1 && answer.ProviderConfigVersion >= 1 &&
		validDisplay(answer.Question, rag.MaxQuestionBytes) && !answer.CreatedAt.IsZero() &&
		!answer.ReconcileAfter.Before(answer.CreatedAt) &&
		(answer.ScopeCollectionID == nil || validIdentifier(*answer.ScopeCollectionID))
}

func validAnswerSources(sources []store.AnswerSource) bool {
	if len(sources) > 8 {
		return false
	}
	for index, source := range sources {
		if source.Position != index+1 || !validIdentifier(source.ChunkID) ||
			!validIdentifier(source.DocumentID) || !validIdentifier(source.RevisionID) ||
			source.ChunkOrdinal < 0 || !validContentHash(source.ContentHash) ||
			!validDisplay(source.DocumentTitle, maxHeaderText) ||
			!utf8.ValidString(source.Content) || source.Content == "" || len(source.Content) > 48<<10 {
			return false
		}
	}
	return true
}

func validAnswerCitations(citations []store.Citation, sourceCount int) bool {
	if len(citations) > 100 {
		return false
	}
	for index, citation := range citations {
		if citation.Occurrence != index+1 || citation.Position < 1 || citation.Position > sourceCount {
			return false
		}
	}
	return true
}

func validContentHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}
