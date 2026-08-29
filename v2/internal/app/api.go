package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mt-hub8/MindWeaver/v2/internal/blob"
	"github.com/mt-hub8/MindWeaver/v2/internal/ingest"
	"github.com/mt-hub8/MindWeaver/v2/internal/lifecycle"
	"github.com/mt-hub8/MindWeaver/v2/internal/localhttp"
	"github.com/mt-hub8/MindWeaver/v2/internal/ollama"
	"github.com/mt-hub8/MindWeaver/v2/internal/rag"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/internal/transport"
	"github.com/mt-hub8/MindWeaver/v2/internal/workbench"
	"github.com/mt-hub8/MindWeaver/v2/platform/version"
)

const (
	apiPrefix         = "/api/v1"
	maxJSONBodyBytes  = int64(16 << 10)
	maxHeaderText     = 1024
	maxIdentifierText = 255
	maxSearchWindow   = 100
	maxSnippetBytes   = 2 << 10
	maxSearchJSON     = 1 << 20
)

type searchHitView struct {
	ChunkID          string  `json:"chunkId"`
	DocumentID       string  `json:"documentId"`
	RevisionID       string  `json:"revisionId"`
	Ordinal          int     `json:"ordinal"`
	Snippet          string  `json:"snippet"`
	SnippetTruncated bool    `json:"snippetTruncated"`
	Rank             float64 `json:"rank"`
}

type searchPage struct {
	Hits       []searchHitView `json:"hits"`
	NextOffset *int            `json:"nextOffset,omitempty"`
}

type documentView struct {
	ID                   string `json:"id"`
	Title                string `json:"title"`
	MediaType            string `json:"mediaType"`
	Status               string `json:"status"`
	ActiveRevisionID     string `json:"activeRevisionId,omitempty"`
	IngestionJobID       string `json:"ingestionJobId,omitempty"`
	IngestionStatus      string `json:"ingestionStatus,omitempty"`
	IngestionErrorCode   string `json:"ingestionErrorCode,omitempty"`
	IngestionAttempt     int    `json:"ingestionAttempt"`
	IngestionMaxAttempts int    `json:"ingestionMaxAttempts"`
	CreatedAt            string `json:"createdAt"`
	UpdatedAt            string `json:"updatedAt"`
	Revision             int64  `json:"revision"`
}

type workbenchAPI interface {
	Upload(context.Context, workbench.UploadRequest) (workbench.UploadResult, error)
	GetJob(context.Context, string) (store.Job, error)
	CancelJob(context.Context, string) error
	RetryDocumentIngestion(context.Context, string, int64) (store.Document, bool, error)
	ListDocumentsPage(context.Context, int, *store.DocumentCursor) (store.DocumentPage, error)
	Search(context.Context, string, int) ([]store.ChunkHit, error)
	SearchCollection(context.Context, string, string, int) ([]store.ChunkHit, error)
	CreateCollectionIdempotent(context.Context, string, string) (store.Collection, bool, error)
	ListCollectionsPage(context.Context, int, *store.CollectionCursor) (store.CollectionPage, error)
	ListCollectionMembersPage(context.Context, string, int, *store.CollectionMemberCursor) (store.CollectionMemberPage, error)
	AddDocumentToCollectionExpected(context.Context, string, string, int64) (store.Collection, bool, error)
	RemoveDocumentFromCollectionExpected(context.Context, string, string, int64) (store.Collection, bool, error)
}

type lifecycleAPI interface {
	TrashExpected(context.Context, string, int64) (store.DocumentLifecycle, error)
	RestoreExpected(context.Context, string, int64) (store.DocumentLifecycle, error)
	PurgeExpected(context.Context, string, int64) (lifecycle.PurgeResult, error)
	PurgeStatus(context.Context, string) (store.DocumentPurgeStatus, error)
	ListPurges(context.Context, int, *store.DocumentPurgeCursor) (store.DocumentPurgePage, error)
}

type backupAPI interface {
	Start(context.Context, string, string) (backupOperationStatus, error)
	Status(string) (backupOperationStatus, error)
	Cancel(string) (backupOperationStatus, error)
}

type API struct {
	service   workbenchAPI
	lifecycle lifecycleAPI
	worker    *ingestionWorker
	rag       *ragRuntime
	backups   backupAPI
	startup   StartupEvidence
	pdfReady  bool
}

func newAPI(service workbenchAPI, lifecycleService lifecycleAPI, worker *ingestionWorker, ragRuntime *ragRuntime, backups backupAPI, startup StartupEvidence, pdfReady bool) *API {
	return &API{service: service, lifecycle: lifecycleService, worker: worker, rag: ragRuntime, backups: backups, startup: startup, pdfReady: pdfReady}
}

// Register installs only fixed endpoint paths. IDs are query/body values, not
// path templates, so localhttp can seal and audit the complete route table.
func (api *API) Register(router *localhttp.Router) error {
	if api == nil || api.service == nil || api.lifecycle == nil || api.worker == nil || api.rag == nil || api.backups == nil || router == nil {
		return errors.New("app: invalid API dependencies")
	}
	routes := []struct {
		method string
		path   string
		serve  http.HandlerFunc
	}{
		{http.MethodGet, apiPrefix + "/runtime", api.runtime},
		{http.MethodGet, apiPrefix + "/diagnostics", api.diagnostics},
		{http.MethodPost, apiPrefix + "/backups", api.createBackup},
		{http.MethodGet, apiPrefix + "/backups/status", api.backupStatus},
		{http.MethodPost, apiPrefix + "/backups/cancel", api.cancelBackup},
		{http.MethodPost, apiPrefix + "/documents/upload", api.upload},
		{http.MethodGet, apiPrefix + "/documents", api.documents},
		{http.MethodPost, apiPrefix + "/documents/retry-ingestion", api.retryDocumentIngestion},
		{http.MethodPost, apiPrefix + "/documents/trash", api.trashDocument},
		{http.MethodPost, apiPrefix + "/documents/restore", api.restoreDocument},
		{http.MethodPost, apiPrefix + "/documents/purge", api.purgeDocument},
		{http.MethodGet, apiPrefix + "/documents/purge-status", api.purgeStatus},
		{http.MethodGet, apiPrefix + "/documents/purges", api.purges},
		{http.MethodGet, apiPrefix + "/jobs", api.job},
		{http.MethodPost, apiPrefix + "/jobs/cancel", api.cancelJob},
		{http.MethodGet, apiPrefix + "/search", api.search},
		{http.MethodPost, apiPrefix + "/collections", api.createCollection},
		{http.MethodGet, apiPrefix + "/collections", api.collections},
		{http.MethodPost, apiPrefix + "/collections/members", api.addCollectionMember},
		{http.MethodGet, apiPrefix + "/collections/members", api.collectionMembers},
		{http.MethodDelete, apiPrefix + "/collections/members", api.removeCollectionMember},
		{http.MethodGet, apiPrefix + "/ollama", api.ollamaConfiguration},
		{http.MethodPut, apiPrefix + "/ollama", api.configureOllama},
		{http.MethodPost, apiPrefix + "/ollama/probe", api.probeOllama},
		{http.MethodPost, apiPrefix + "/conversations", api.createConversation},
		{http.MethodGet, apiPrefix + "/conversations", api.conversations},
		{http.MethodDelete, apiPrefix + "/conversations", api.deleteConversation},
		{http.MethodGet, apiPrefix + "/conversations/messages", api.conversationMessages},
		{http.MethodPost, apiPrefix + "/ask", api.ask},
		{http.MethodGet, apiPrefix + "/answers", api.answer},
	}
	for _, route := range routes {
		if err := router.HandleFunc(route.method, route.path, route.serve); err != nil {
			return fmt.Errorf("app: register %s %s: %w", route.method, route.path, err)
		}
	}
	return nil
}

func (api *API) createBackup(response http.ResponseWriter, request *http.Request) {
	if !noUnexpectedQuery(request) {
		api.problem(response, request, transport.CodeInvalidArgument, "创建备份不能包含查询参数。")
		return
	}
	idempotency, ok := singleHeader(request.Header, "Idempotency-Key")
	if !ok || !validOpaque(idempotency, maxBackupIdempotencyBytes) {
		api.problem(response, request, transport.CodeInvalidArgument, "Idempotency-Key 必须是 1 到 256 字节的非空值。")
		return
	}
	var input struct {
		Destination string `json:"destination"`
	}
	if err := decodeStrictJSON(request, &input); err != nil || !validAbsoluteBackupPath(input.Destination) {
		api.problem(response, request, transport.CodeInvalidArgument, "备份目标必须是已有固定本地盘目录下的新绝对路径。")
		return
	}
	status, err := api.backups.Start(request.Context(), idempotency, input.Destination)
	if err != nil {
		code, detail := classifyBackupControlError(err)
		api.problem(response, request, code, detail)
		return
	}
	writeJSON(response, http.StatusAccepted, status)
}

func (api *API) backupStatus(response http.ResponseWriter, request *http.Request) {
	if !noUnexpectedQuery(request, "operationId") {
		api.problem(response, request, transport.CodeInvalidArgument, "备份状态查询包含未知参数。")
		return
	}
	operationID, ok := oneQuery(request, "operationId")
	if !ok || !validBackupOperationID(operationID) {
		api.problem(response, request, transport.CodeInvalidArgument, "必须提供有效的备份 operationId。")
		return
	}
	status, err := api.backups.Status(operationID)
	if err != nil {
		code, detail := classifyBackupControlError(err)
		api.problem(response, request, code, detail)
		return
	}
	writeJSON(response, http.StatusOK, status)
}

func (api *API) cancelBackup(response http.ResponseWriter, request *http.Request) {
	if !noUnexpectedQuery(request) {
		api.problem(response, request, transport.CodeInvalidArgument, "取消备份不能包含查询参数。")
		return
	}
	var input struct {
		OperationID string `json:"operationId"`
	}
	if err := decodeStrictJSON(request, &input); err != nil || !validBackupOperationID(input.OperationID) {
		api.problem(response, request, transport.CodeInvalidArgument, "取消备份必须包含有效且唯一的 operationId。")
		return
	}
	status, err := api.backups.Cancel(input.OperationID)
	if err != nil {
		code, detail := classifyBackupControlError(err)
		api.problem(response, request, code, detail)
		return
	}
	writeJSON(response, http.StatusAccepted, status)
}

func (api *API) runtime(response http.ResponseWriter, request *http.Request) {
	if !noUnexpectedQuery(request) {
		api.problem(response, request, transport.CodeInvalidArgument, "运行时请求不能包含查询参数。")
		return
	}
	modelStatus := api.modelStatus(request.Context())
	writeJSON(response, http.StatusOK, struct {
		Version       string `json:"version"`
		State         string `json:"state"`
		ModelStatus   string `json:"modelStatus"`
		PDFAvailable  bool   `json:"pdfAvailable"`
		ConfigCreated bool   `json:"configCreated"`
	}{Version: version.String(), State: "ready", ModelStatus: modelStatus, PDFAvailable: api.pdfReady, ConfigCreated: api.startup.ConfigCreated})
}

func (api *API) diagnostics(response http.ResponseWriter, request *http.Request) {
	if !noUnexpectedQuery(request) {
		api.problem(response, request, transport.CodeInvalidArgument, "诊断请求不能包含查询参数。")
		return
	}
	writeJSON(response, http.StatusOK, struct {
		VaultStatus              string `json:"vaultStatus"`
		DatabaseStatus           string `json:"databaseStatus"`
		WorkerStatus             string `json:"workerStatus"`
		ModelStatus              string `json:"modelStatus"`
		PDFAvailable             bool   `json:"pdfAvailable"`
		RecoveredJobs            int64  `json:"recoveredJobs"`
		CleanedStagingFiles      int    `json:"cleanedStagingFiles"`
		SweptBlobCandidates      int    `json:"sweptBlobCandidates"`
		ReconciledPendingAnswers int64  `json:"reconciledPendingAnswers"`
	}{
		VaultStatus: "locked_by_this_process", DatabaseStatus: "ready",
		WorkerStatus: api.worker.Status(), ModelStatus: api.modelStatus(request.Context()),
		PDFAvailable:  api.pdfReady,
		RecoveredJobs: api.startup.RecoveredJobs, CleanedStagingFiles: api.startup.CleanedStagingFiles,
		SweptBlobCandidates:      api.startup.SweptBlobCandidates,
		ReconciledPendingAnswers: api.startup.ReconciledPendingAnswers,
	})
}

func (api *API) modelStatus(ctx context.Context) string {
	config, err := api.rag.service.GetActiveOllamaConfig(ctx)
	switch {
	case err == nil && config.Timeout >= time.Millisecond && config.Timeout <= rag.MaxOllamaTimeout:
		return "configured"
	case err == nil:
		return "invalid"
	case errors.Is(err, store.ErrOllamaNotConfigured):
		return "unconfigured"
	default:
		return "unavailable"
	}
}

func (api *API) upload(response http.ResponseWriter, request *http.Request) {
	if !noUnexpectedQuery(request) {
		api.problem(response, request, transport.CodeInvalidArgument, "上传请求不能包含查询参数。")
		return
	}
	if !exactMediaType(request.Header, "application/octet-stream") || request.Header.Get("Content-Encoding") != "" {
		api.problem(response, request, transport.CodeInvalidArgument, "上传请求必须使用 application/octet-stream 且不能压缩。")
		return
	}
	idempotency, ok := singleHeader(request.Header, "Idempotency-Key")
	if !ok || !validOpaque(idempotency, 256) {
		api.problem(response, request, transport.CodeInvalidArgument, "Idempotency-Key 必须是 1 到 256 字节的非空值。")
		return
	}
	title, ok := decodedTextHeader(request.Header, "X-MindWeaver-Title-B64", maxHeaderText)
	if !ok || !validDisplay(title, maxHeaderText) {
		api.problem(response, request, transport.CodeInvalidArgument, "文档标题必须是 1 到 1024 字节的有效文本。")
		return
	}
	filename, ok := decodedTextHeader(request.Header, "X-MindWeaver-Filename-B64", maxHeaderText)
	if !ok || !validFilename(filename) {
		api.problem(response, request, transport.CodeInvalidArgument, "文件名必须是普通的 TXT、Markdown 或 PDF 文件名。")
		return
	}
	result, err := api.service.Upload(request.Context(), workbench.UploadRequest{
		IdempotencyKey: idempotency,
		Title:          title,
		Filename:       filename,
		Source:         request.Body,
	})
	if err != nil {
		code, detail := classifyError(err)
		api.problem(response, request, code, detail)
		return
	}
	api.worker.Wake()
	status := http.StatusOK
	if result.Created {
		status = http.StatusAccepted
	}
	writeJSON(response, status, struct {
		DocumentID string `json:"documentId"`
		RevisionID string `json:"revisionId"`
		JobID      string `json:"jobId"`
		Created    bool   `json:"created"`
	}{result.DocumentID, result.RevisionID, result.JobID, result.Created})
}

func (api *API) documents(response http.ResponseWriter, request *http.Request) {
	if !noUnexpectedQuery(request, "limit", "cursor") {
		api.problem(response, request, transport.CodeInvalidArgument, "文档列表包含未知查询参数。")
		return
	}
	limit, err := queryLimit(request, 50)
	if err != nil {
		api.problem(response, request, transport.CodeInvalidArgument, "limit 必须是 1 到 100 的整数。")
		return
	}
	var after *store.DocumentCursor
	if raw, exists := optionalQuery(request, "cursor"); exists {
		stamp, id, err := decodeAPICursor(raw, documentCursorKind, "")
		if err != nil {
			api.problem(response, request, transport.CodeInvalidArgument, "cursor 不是有效的文档目录游标。")
			return
		}
		after = &store.DocumentCursor{CreatedAt: stamp, ID: id}
	}
	page, err := api.service.ListDocumentsPage(request.Context(), limit, after)
	if err != nil {
		code, detail := classifyError(err)
		api.problem(response, request, code, detail)
		return
	}
	items := make([]documentView, 0, len(page.Documents))
	for _, item := range page.Documents {
		items = append(items, viewDocument(item))
	}
	var nextCursor string
	if page.Next != nil {
		nextCursor, err = encodeAPICursor(documentCursorKind, "", page.Next.CreatedAt, page.Next.ID)
		if err != nil {
			api.problem(response, request, transport.CodeInternal, "无法编码文档目录游标。")
			return
		}
	}
	writeJSON(response, http.StatusOK, struct {
		Documents  []documentView `json:"documents"`
		NextCursor string         `json:"nextCursor,omitempty"`
	}{Documents: items, NextCursor: nextCursor})
}

func (api *API) job(response http.ResponseWriter, request *http.Request) {
	if !noUnexpectedQuery(request, "id") {
		api.problem(response, request, transport.CodeInvalidArgument, "任务查询包含未知参数。")
		return
	}
	id, ok := oneQuery(request, "id")
	if !ok || !validIdentifier(id) {
		api.problem(response, request, transport.CodeInvalidArgument, "必须提供有效的任务 ID。")
		return
	}
	job, err := api.service.GetJob(request.Context(), id)
	if err != nil {
		code, detail := classifyError(err)
		api.problem(response, request, code, detail)
		return
	}
	writeJSON(response, http.StatusOK, struct {
		Job jobView `json:"job"`
	}{Job: viewJob(job)})
}

func (api *API) cancelJob(response http.ResponseWriter, request *http.Request) {
	var input struct {
		ID string `json:"id"`
	}
	if err := decodeStrictJSON(request, &input); err != nil || !validIdentifier(input.ID) {
		api.problem(response, request, transport.CodeInvalidArgument, "取消请求必须包含有效且唯一的 id 字段。")
		return
	}
	if err := api.service.CancelJob(request.Context(), input.ID); err != nil {
		code, detail := classifyError(err)
		api.problem(response, request, code, detail)
		return
	}
	// SQLite is authoritative: only propagate the in-process latency hint after
	// the cancellation request is durable. A claim which has not registered its
	// child context yet observes the same flag in ingestionWorker.runClaimed.
	if api.worker != nil {
		api.worker.CancelActive(input.ID)
	}
	response.WriteHeader(http.StatusNoContent)
}

func (api *API) retryDocumentIngestion(response http.ResponseWriter, request *http.Request) {
	if !noUnexpectedQuery(request) {
		api.problem(response, request, transport.CodeInvalidArgument, "摄取重试请求不能包含查询参数。")
		return
	}
	var input struct {
		DocumentID       string `json:"documentId"`
		ExpectedRevision *int64 `json:"expectedRevision"`
	}
	if err := decodeStrictJSON(request, &input); err != nil || !validIdentifier(input.DocumentID) || input.ExpectedRevision == nil || *input.ExpectedRevision < 0 {
		api.problem(response, request, transport.CodeInvalidArgument, "摄取重试必须包含有效且唯一的 documentId 与 expectedRevision。")
		return
	}
	document, changed, err := api.service.RetryDocumentIngestion(request.Context(), input.DocumentID, *input.ExpectedRevision)
	if err != nil {
		code, detail := classifyError(err)
		api.problem(response, request, code, detail)
		return
	}
	api.worker.Wake()
	writeJSON(response, http.StatusOK, struct {
		Document documentView `json:"document"`
		Changed  bool         `json:"changed"`
	}{Document: viewDocument(document), Changed: changed})
}

func (api *API) search(response http.ResponseWriter, request *http.Request) {
	if !noUnexpectedQuery(request, "q", "limit", "offset", "collection_id") {
		api.problem(response, request, transport.CodeInvalidArgument, "搜索请求包含未知查询参数。")
		return
	}
	query, ok := oneQuery(request, "q")
	if !ok || !validSearch(query) {
		api.problem(response, request, transport.CodeInvalidArgument, "搜索词必须包含至少 3 个字符，且不超过 1024 字节。")
		return
	}
	limit, err := queryLimit(request, 20)
	if err != nil {
		api.problem(response, request, transport.CodeInvalidArgument, "limit 必须是 1 到 100 的整数。")
		return
	}
	offset, err := queryOffset(request)
	if err != nil {
		api.problem(response, request, transport.CodeInvalidArgument, "offset 必须是 0 到 99 的整数。")
		return
	}
	// Fetch one extra ordered hit whenever the bounded 100-result window has
	// room. That probe makes nextOffset exact without adding a count query or
	// changing the store contract. Replaying q/scope/offset over unchanged data
	// yields the same page because the store order has a deterministic tie-break.
	fetchLimit := offset + limit + 1
	if fetchLimit > maxSearchWindow {
		fetchLimit = maxSearchWindow
	}
	collectionID, hasCollection := optionalQuery(request, "collection_id")
	var hits []store.ChunkHit
	if hasCollection {
		if !validIdentifier(collectionID) {
			api.problem(response, request, transport.CodeInvalidArgument, "集合 ID 无效。")
			return
		}
		hits, err = api.service.SearchCollection(request.Context(), collectionID, query, fetchLimit)
	} else {
		hits, err = api.service.Search(request.Context(), query, fetchLimit)
	}
	if err != nil {
		code, detail := classifyError(err)
		api.problem(response, request, code, detail)
		return
	}
	if len(hits) > fetchLimit {
		hits = hits[:fetchLimit]
	}
	if offset > len(hits) {
		offset = len(hits)
	}
	available := hits[offset:]
	if len(available) > limit {
		available = available[:limit+1]
	}
	body, err := encodeSearchPage(available, offset, limit)
	if err != nil {
		api.problem(response, request, transport.CodeInternal, "无法安全编码搜索结果。")
		return
	}
	writeEncodedJSON(response, http.StatusOK, body)
}

func encodeSearchPage(hits []store.ChunkHit, offset, limit int) ([]byte, error) {
	pageCount := len(hits)
	if pageCount > limit {
		pageCount = limit
	}
	items := make([]searchHitView, 0, pageCount)
	for _, hit := range hits[:pageCount] {
		snippet, truncated := boundedSnippet(hit.Content)
		item := searchHitView{
			ChunkID: hit.ChunkID, DocumentID: hit.DocumentID, RevisionID: hit.RevisionID,
			Ordinal: hit.Ordinal, Snippet: snippet, SnippetTruncated: truncated, Rank: hit.Rank,
		}
		trialItems := append(items, item)
		trialOffset := offset + len(trialItems)
		trial, err := json.Marshal(searchPage{Hits: trialItems, NextOffset: &trialOffset})
		if err != nil {
			return nil, err
		}
		if len(trial)+1 > maxSearchJSON {
			break
		}
		items = trialItems
	}
	if len(items) == 0 && pageCount != 0 {
		return nil, errors.New("one bounded search hit exceeds response budget")
	}
	result := searchPage{Hits: items}
	if len(items) < len(hits) {
		next := offset + len(items)
		result.NextOffset = &next
	}
	body, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	body = append(body, '\n')
	if len(body) > maxSearchJSON {
		return nil, errors.New("search response exceeds encoding budget")
	}
	return body, nil
}

func boundedSnippet(content string) (string, bool) {
	normalized := strings.ToValidUTF8(content, "\uFFFD")
	changed := normalized != content
	if len(normalized) <= maxSnippetBytes {
		return normalized, changed
	}
	end := maxSnippetBytes
	for end > 0 && !utf8.ValidString(normalized[:end]) {
		end--
	}
	return normalized[:end], true
}

func (api *API) createCollection(response http.ResponseWriter, request *http.Request) {
	if !noUnexpectedQuery(request) {
		api.problem(response, request, transport.CodeInvalidArgument, "创建集合不能包含查询参数。")
		return
	}
	idempotency, ok := singleHeader(request.Header, "Idempotency-Key")
	if !ok || !validOpaque(idempotency, 256) {
		api.problem(response, request, transport.CodeInvalidArgument, "Idempotency-Key 必须是 1 到 256 字节的非空值。")
		return
	}
	var input struct {
		Name string `json:"name"`
	}
	if err := decodeStrictJSON(request, &input); err != nil || !validDisplay(input.Name, maxHeaderText) {
		api.problem(response, request, transport.CodeInvalidArgument, "集合名称必须是 1 到 1024 字节的有效文本。")
		return
	}
	collection, created, err := api.service.CreateCollectionIdempotent(request.Context(), idempotency, input.Name)
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
		Collection collectionView `json:"collection"`
		Created    bool           `json:"created"`
	}{Collection: viewCollection(collection), Created: created})
}

func (api *API) collections(response http.ResponseWriter, request *http.Request) {
	if !noUnexpectedQuery(request, "limit", "cursor") {
		api.problem(response, request, transport.CodeInvalidArgument, "集合列表包含未知查询参数。")
		return
	}
	limit, err := queryLimit(request, 50)
	if err != nil {
		api.problem(response, request, transport.CodeInvalidArgument, "limit 必须是 1 到 100 的整数。")
		return
	}
	var after *store.CollectionCursor
	if raw, exists := optionalQuery(request, "cursor"); exists {
		stamp, id, err := decodeAPICursor(raw, collectionCursorKind, "")
		if err != nil {
			api.problem(response, request, transport.CodeInvalidArgument, "cursor 不是有效的集合目录游标。")
			return
		}
		after = &store.CollectionCursor{CreatedAt: stamp, ID: id}
	}
	page, err := api.service.ListCollectionsPage(request.Context(), limit, after)
	if err != nil {
		code, detail := classifyError(err)
		api.problem(response, request, code, detail)
		return
	}
	items := make([]collectionView, 0, len(page.Collections))
	for _, item := range page.Collections {
		items = append(items, viewCollection(item))
	}
	var nextCursor string
	if page.Next != nil {
		nextCursor, err = encodeAPICursor(collectionCursorKind, "", page.Next.CreatedAt, page.Next.ID)
		if err != nil {
			api.problem(response, request, transport.CodeInternal, "无法编码集合目录游标。")
			return
		}
	}
	writeJSON(response, http.StatusOK, struct {
		Collections []collectionView `json:"collections"`
		NextCursor  string           `json:"nextCursor,omitempty"`
	}{Collections: items, NextCursor: nextCursor})
}

func (api *API) collectionMembers(response http.ResponseWriter, request *http.Request) {
	if !noUnexpectedQuery(request, "collection_id", "limit", "cursor") {
		api.problem(response, request, transport.CodeInvalidArgument, "集合成员列表包含未知查询参数。")
		return
	}
	collectionID, ok := oneQuery(request, "collection_id")
	if !ok || !validIdentifier(collectionID) {
		api.problem(response, request, transport.CodeInvalidArgument, "必须提供有效的集合 ID。")
		return
	}
	limit, err := queryLimit(request, 50)
	if err != nil {
		api.problem(response, request, transport.CodeInvalidArgument, "limit 必须是 1 到 100 的整数。")
		return
	}
	var after *store.CollectionMemberCursor
	if raw, exists := optionalQuery(request, "cursor"); exists {
		stamp, id, err := decodeAPICursor(raw, memberCursorKind, collectionID)
		if err != nil {
			api.problem(response, request, transport.CodeInvalidArgument, "cursor 不是该集合的有效成员游标。")
			return
		}
		after = &store.CollectionMemberCursor{AddedAt: stamp, DocumentID: id}
	}
	page, err := api.service.ListCollectionMembersPage(request.Context(), collectionID, limit, after)
	if err != nil {
		code, detail := classifyError(err)
		api.problem(response, request, code, detail)
		return
	}
	members := make([]collectionMemberView, 0, len(page.Members))
	for _, member := range page.Members {
		members = append(members, collectionMemberView{
			Document: viewDocument(member.Document),
			AddedAt:  formatTime(member.AddedAt),
		})
	}
	var nextCursor string
	if page.Next != nil {
		nextCursor, err = encodeAPICursor(memberCursorKind, collectionID, page.Next.AddedAt, page.Next.DocumentID)
		if err != nil {
			api.problem(response, request, transport.CodeInternal, "无法编码集合成员游标。")
			return
		}
	}
	writeJSON(response, http.StatusOK, struct {
		Collection collectionView         `json:"collection"`
		Members    []collectionMemberView `json:"members"`
		NextCursor string                 `json:"nextCursor,omitempty"`
	}{Collection: viewCollection(page.Collection), Members: members, NextCursor: nextCursor})
}

func (api *API) addCollectionMember(response http.ResponseWriter, request *http.Request) {
	api.mutateCollectionMember(response, request, true)
}

func (api *API) removeCollectionMember(response http.ResponseWriter, request *http.Request) {
	api.mutateCollectionMember(response, request, false)
}

func (api *API) mutateCollectionMember(response http.ResponseWriter, request *http.Request, add bool) {
	if !noUnexpectedQuery(request) {
		api.problem(response, request, transport.CodeInvalidArgument, "集合成员写入不能包含查询参数。")
		return
	}
	var input struct {
		CollectionID     string `json:"collectionId"`
		DocumentID       string `json:"documentId"`
		ExpectedRevision *int64 `json:"expectedRevision"`
	}
	if err := decodeStrictJSON(request, &input); err != nil || !validIdentifier(input.CollectionID) || !validIdentifier(input.DocumentID) || input.ExpectedRevision == nil || *input.ExpectedRevision < 0 {
		api.problem(response, request, transport.CodeInvalidArgument, "集合成员请求必须包含有效且唯一的 collectionId、documentId 与 expectedRevision。")
		return
	}
	var collection store.Collection
	var changed bool
	var err error
	if add {
		collection, changed, err = api.service.AddDocumentToCollectionExpected(request.Context(), input.CollectionID, input.DocumentID, *input.ExpectedRevision)
	} else {
		collection, changed, err = api.service.RemoveDocumentFromCollectionExpected(request.Context(), input.CollectionID, input.DocumentID, *input.ExpectedRevision)
	}
	if err != nil {
		code, detail := classifyError(err)
		api.problem(response, request, code, detail)
		return
	}
	writeJSON(response, http.StatusOK, struct {
		Collection collectionView `json:"collection"`
		Changed    bool           `json:"changed"`
	}{Collection: viewCollection(collection), Changed: changed})
}

type documentMutationInput struct {
	DocumentID       string `json:"documentId"`
	ExpectedRevision int64  `json:"expectedRevision"`
}

func (api *API) trashDocument(response http.ResponseWriter, request *http.Request) {
	input, ok := api.decodeDocumentMutation(response, request)
	if !ok {
		return
	}
	state, err := api.lifecycle.TrashExpected(request.Context(), input.DocumentID, input.ExpectedRevision)
	if err != nil {
		code, detail := classifyError(err)
		api.problem(response, request, code, detail)
		return
	}
	writeJSON(response, http.StatusOK, struct {
		Document lifecycleView `json:"document"`
	}{Document: viewLifecycle(state)})
}

func (api *API) restoreDocument(response http.ResponseWriter, request *http.Request) {
	input, ok := api.decodeDocumentMutation(response, request)
	if !ok {
		return
	}
	state, err := api.lifecycle.RestoreExpected(request.Context(), input.DocumentID, input.ExpectedRevision)
	if err != nil {
		code, detail := classifyError(err)
		api.problem(response, request, code, detail)
		return
	}
	writeJSON(response, http.StatusOK, struct {
		Document lifecycleView `json:"document"`
	}{Document: viewLifecycle(state)})
}

func (api *API) purgeDocument(response http.ResponseWriter, request *http.Request) {
	input, ok := api.decodeDocumentMutation(response, request)
	if !ok {
		return
	}
	result, err := api.lifecycle.PurgeExpected(request.Context(), input.DocumentID, input.ExpectedRevision)
	if err != nil && !result.DatabaseDeleted {
		code, detail := classifyError(err)
		api.problem(response, request, code, detail)
		return
	}
	state := "complete"
	status := http.StatusOK
	if !result.Complete {
		state = "pending"
		if err != nil {
			state = "failed"
		}
		status = http.StatusAccepted
	}
	writeJSON(response, status, purgeResultView{
		DocumentID: result.DocumentID, State: state,
		DatabaseDeleted: result.DatabaseDeleted, Complete: result.Complete,
		AllCandidateObjectsRemoved: result.AllCandidateObjectsRemoved,
		DeletedBlobCount:           len(result.DeletedBlobIDs), RetainedSharedCount: len(result.RetainedSharedIDs),
		PendingBlobCount: len(result.PendingBlobIDs), DeletedJobCount: len(result.DeletedJobIDs),
	})
}

func (api *API) purgeStatus(response http.ResponseWriter, request *http.Request) {
	if !noUnexpectedQuery(request, "id") {
		api.problem(response, request, transport.CodeInvalidArgument, "清理状态查询包含未知参数。")
		return
	}
	id, ok := oneQuery(request, "id")
	if !ok || !validIdentifier(id) {
		api.problem(response, request, transport.CodeInvalidArgument, "必须提供有效的文档 ID。")
		return
	}
	status, err := api.lifecycle.PurgeStatus(request.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		api.problem(response, request, transport.CodeNotFound, "该文档当前没有进行中的清理；这不是永久删除成功凭据。")
		return
	}
	if err != nil {
		code, detail := classifyError(err)
		api.problem(response, request, code, detail)
		return
	}
	writeJSON(response, http.StatusOK, viewPurgeStatus(status))
}

func (api *API) purges(response http.ResponseWriter, request *http.Request) {
	if !noUnexpectedQuery(request, "limit", "cursor") {
		api.problem(response, request, transport.CodeInvalidArgument, "清理列表包含未知查询参数。")
		return
	}
	limit, err := queryLimit(request, 50)
	if err != nil {
		api.problem(response, request, transport.CodeInvalidArgument, "limit 必须是 1 到 100 的整数。")
		return
	}
	var after *store.DocumentPurgeCursor
	if raw, exists := optionalQuery(request, "cursor"); exists {
		stamp, id, err := decodeAPICursor(raw, purgeCursorKind, "")
		if err != nil {
			api.problem(response, request, transport.CodeInvalidArgument, "cursor 不是有效的清理列表游标。")
			return
		}
		after = &store.DocumentPurgeCursor{RequestedAt: stamp, DocumentID: id}
	}
	page, err := api.lifecycle.ListPurges(request.Context(), limit, after)
	if err != nil {
		code, detail := classifyError(err)
		api.problem(response, request, code, detail)
		return
	}
	items := make([]purgeStatusView, 0, len(page.Purges))
	for _, item := range page.Purges {
		items = append(items, purgeStatusView{
			DocumentID: item.DocumentID, Status: item.Status, LastErrorCode: item.LastErrorCode,
			RemainingBlobCount: item.RemainingBlobCount,
			RequestedAt:        formatTime(item.RequestedAt), UpdatedAt: formatTime(item.UpdatedAt),
		})
	}
	var nextCursor string
	if page.Next != nil {
		nextCursor, err = encodeAPICursor(purgeCursorKind, "", page.Next.RequestedAt, page.Next.DocumentID)
		if err != nil {
			api.problem(response, request, transport.CodeInternal, "无法编码清理列表游标。")
			return
		}
	}
	writeJSON(response, http.StatusOK, struct {
		Purges     []purgeStatusView `json:"purges"`
		NextCursor string            `json:"nextCursor,omitempty"`
	}{Purges: items, NextCursor: nextCursor})
}

func (api *API) decodeDocumentMutation(response http.ResponseWriter, request *http.Request) (documentMutationInput, bool) {
	if !noUnexpectedQuery(request) {
		api.problem(response, request, transport.CodeInvalidArgument, "文档生命周期写入不能包含查询参数。")
		return documentMutationInput{}, false
	}
	var wire struct {
		DocumentID       string `json:"documentId"`
		ExpectedRevision *int64 `json:"expectedRevision"`
	}
	if err := decodeStrictJSON(request, &wire); err != nil || !validIdentifier(wire.DocumentID) || wire.ExpectedRevision == nil || *wire.ExpectedRevision < 0 {
		api.problem(response, request, transport.CodeInvalidArgument, "生命周期请求必须包含有效且唯一的 documentId 与 expectedRevision。")
		return documentMutationInput{}, false
	}
	return documentMutationInput{DocumentID: wire.DocumentID, ExpectedRevision: *wire.ExpectedRevision}, true
}

type jobView struct {
	ID              string `json:"id"`
	Kind            string `json:"kind"`
	Status          string `json:"status"`
	Attempt         int    `json:"attempt"`
	MaxAttempts     int    `json:"maxAttempts"`
	CancelRequested bool   `json:"cancelRequested"`
	ErrorCode       string `json:"errorCode,omitempty"`
	UpdatedAt       string `json:"updatedAt"`
}

type collectionView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
	Revision  int64  `json:"revision"`
}

type collectionMemberView struct {
	Document documentView `json:"document"`
	AddedAt  string       `json:"addedAt"`
}

type lifecycleView struct {
	DocumentID       string  `json:"documentId"`
	Status           string  `json:"status"`
	Revision         int64   `json:"revision"`
	TrashedAt        *string `json:"trashedAt,omitempty"`
	PurgeRequestedAt *string `json:"purgeRequestedAt,omitempty"`
}

type purgeResultView struct {
	DocumentID                 string `json:"documentId"`
	State                      string `json:"state"`
	DatabaseDeleted            bool   `json:"databaseDeleted"`
	Complete                   bool   `json:"complete"`
	AllCandidateObjectsRemoved bool   `json:"allCandidateObjectsRemoved"`
	DeletedBlobCount           int    `json:"deletedBlobCount"`
	RetainedSharedCount        int    `json:"retainedSharedCount"`
	PendingBlobCount           int    `json:"pendingBlobCount"`
	DeletedJobCount            int    `json:"deletedJobCount"`
}

type purgeStatusView struct {
	DocumentID         string `json:"documentId"`
	Status             string `json:"status"`
	LastErrorCode      string `json:"lastErrorCode,omitempty"`
	RemainingBlobCount int    `json:"remainingBlobCount"`
	RequestedAt        string `json:"requestedAt"`
	UpdatedAt          string `json:"updatedAt"`
}

func viewCollection(collection store.Collection) collectionView {
	return collectionView{
		ID: collection.ID, Name: collection.Name,
		CreatedAt: formatTime(collection.CreatedAt),
		UpdatedAt: formatTime(collection.UpdatedAt), Revision: collection.Revision,
	}
}

func viewDocument(document store.Document) documentView {
	return documentView{
		ID: document.ID, Title: document.Title, MediaType: document.MediaType, Status: document.Status,
		ActiveRevisionID: document.ActiveRevisionID, IngestionJobID: document.IngestionJobID,
		IngestionStatus: string(document.IngestionStatus), IngestionErrorCode: document.IngestionErrorCode,
		IngestionAttempt: document.IngestionAttempt, IngestionMaxAttempts: document.IngestionMaxAttempts,
		CreatedAt: formatTime(document.CreatedAt), UpdatedAt: formatTime(document.UpdatedAt), Revision: document.Revision,
	}
}

func viewLifecycle(state store.DocumentLifecycle) lifecycleView {
	view := lifecycleView{DocumentID: state.DocumentID, Status: state.Status, Revision: state.Revision}
	if state.TrashedAt != nil {
		value := formatTime(*state.TrashedAt)
		view.TrashedAt = &value
	}
	if state.PurgeRequestedAt != nil {
		value := formatTime(*state.PurgeRequestedAt)
		view.PurgeRequestedAt = &value
	}
	return view
}

func viewPurgeStatus(status store.DocumentPurgeStatus) purgeStatusView {
	return purgeStatusView{
		DocumentID: status.DocumentID, Status: status.Status, LastErrorCode: status.LastErrorCode,
		RemainingBlobCount: len(status.RemainingBlobIDs),
		RequestedAt:        formatTime(status.RequestedAt), UpdatedAt: formatTime(status.UpdatedAt),
	}
}

func formatTime(value time.Time) string {
	return value.Format("2006-01-02T15:04:05.000000Z07:00")
}

func viewJob(job store.Job) jobView {
	return jobView{
		ID: job.ID, Kind: job.Kind, Status: string(job.Status), Attempt: job.Attempt,
		MaxAttempts: job.MaxAttempts, CancelRequested: job.CancelRequested,
		ErrorCode: job.ErrorCode, UpdatedAt: job.UpdatedAt.Format("2006-01-02T15:04:05.000000Z07:00"),
	}
}

func (api *API) problem(response http.ResponseWriter, request *http.Request, code transport.ErrorCode, detail string) {
	problem := transport.NewProblem(code, detail, newRequestID())
	problem.Instance = request.URL.Path
	localhttp.WriteProblem(response, problem)
}

func classifyError(err error) (transport.ErrorCode, string) {
	var maxBody *http.MaxBytesError
	switch {
	case errors.Is(err, store.ErrNotFound):
		return transport.CodeNotFound, "请求的本地资源不存在。"
	case errors.Is(err, store.ErrIdempotencyConflict):
		return transport.CodeConflict, "该幂等键已经用于不同的请求内容。"
	case errors.Is(err, store.ErrRevisionConflict):
		return transport.CodeConflict, "资源已被其他操作更新；请刷新后按最新 revision 重试。"
	case errors.Is(err, store.ErrConversationIdempotency), errors.Is(err, store.ErrAskIdempotency):
		return transport.CodeConflict, "该幂等键已经用于不同的会话或 Ask 请求。"
	case errors.Is(err, store.ErrConversationRevision):
		return transport.CodeConflict, "会话已被其他操作更新；请刷新历史后按最新 revision 重试。"
	case errors.Is(err, store.ErrConversationBusy):
		return transport.CodeConflict, "会话仍有等待终态的 Ask；请等待完成后再继续提问或删除。"
	case errors.Is(err, store.ErrProviderConfigRevision):
		return transport.CodeConflict, "模型配置已被更新；请刷新后按最新 version 重试。"
	case errors.Is(err, store.ErrOllamaNotConfigured):
		return transport.CodeConflict, "尚未配置本地 Ollama 模型；其它工作台功能仍可使用。"
	case errors.Is(err, store.ErrHistoryCursorInvalid):
		return transport.CodeInvalidArgument, "历史游标无效或已不属于当前目录。"
	case errors.Is(err, store.ErrHistoryItemTooLarge):
		return transport.CodeResourceLimit, "单条历史记录超过安全响应上限。"
	case errors.Is(err, rag.ErrQuestionTooShort), errors.Is(err, ollama.ErrInvalidConfig), errors.Is(err, ollama.ErrInvalidRequest):
		return transport.CodeInvalidArgument, "模型或 Ask 请求参数无效。"
	case errors.Is(err, errAskQuiescing):
		return transport.CodeServiceUnavailable, "应用正在安全关闭，不能接受新的 Ask。"
	case errors.Is(err, ollama.ErrUnavailable), errors.Is(err, ollama.ErrOutcomeUncertain), errors.Is(err, ollama.ErrProtocol), errors.Is(err, ollama.ErrResponseTooLarge), errors.Is(err, ollama.ErrRequestTooLarge):
		return transport.CodeServiceUnavailable, "本地模型不可用或返回了无效响应。"
	case errors.Is(err, store.ErrCollectionNameConflict):
		return transport.CodeConflict, "同名集合已经存在。"
	case errors.Is(err, store.ErrLifecycleConflict):
		return transport.CodeConflict, "文档当前状态不允许该操作；请刷新后重试。"
	case errors.Is(err, store.ErrIngestionRetryConflict):
		return transport.CodeConflict, "当前摄取状态不能重试；请刷新文档状态。"
	case errors.Is(err, store.ErrPurgeBlockedByAnswers):
		return transport.CodeConflict, "已保存回答仍引用该文档；请先处理相关会话。"
	case errors.Is(err, store.ErrQueryTooShort), errors.Is(err, ingest.ErrUnsupportedFormat):
		return transport.CodeInvalidArgument, "请求参数无效。"
	case store.IsRetryableContention(err):
		return transport.CodeServiceUnavailable, "本地数据库暂时繁忙；请稍后按原请求重试。"
	case errors.Is(err, workbench.ErrPDFUnavailable):
		return transport.CodeServiceUnavailable, "PDF 隔离解析器当前不可用；TXT 和 Markdown 仍可正常使用。"
	case errors.Is(err, ingest.ErrChunkLimit):
		return transport.CodeResourceLimit, "源文本无法在安全分块上限内处理。"
	case errors.Is(err, blob.ErrTooLarge), errors.Is(err, ingest.ErrSourceTooLarge), errors.As(err, &maxBody):
		return transport.CodeResourceLimit, "上传内容超过 4 MiB 限制。"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return transport.CodeServiceUnavailable, "操作已取消或超时，请确认状态后重试。"
	default:
		return transport.CodeInternal, "本地操作失败；可以在诊断面板检查运行状态。"
	}
}

func classifyBackupControlError(err error) (transport.ErrorCode, string) {
	switch {
	case errors.Is(err, errBackupDestinationInvalid):
		return transport.CodeInvalidArgument, "备份目标必须是已有固定本地盘目录下的新绝对路径。"
	case errors.Is(err, errBackupIdempotencyConflict):
		return transport.CodeConflict, "该幂等键已经用于不同的备份目标。"
	case errors.Is(err, errBackupBusy):
		return transport.CodeConflict, "当前已有一个备份正在运行。"
	case errors.Is(err, errBackupNotFound):
		return transport.CodeNotFound, "备份操作不存在或其进程内状态已过期。"
	case errors.Is(err, errBackupQuiescing):
		return transport.CodeServiceUnavailable, "应用正在安全关闭，不能接受新的备份。"
	case errors.Is(err, errBackupHistoryFull):
		return transport.CodeResourceLimit, "本次运行已达到备份操作历史上限；请重启应用后再试。"
	default:
		return transport.CodeInternal, "本地备份控制失败；可以在诊断面板检查运行状态。"
	}
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		http.Error(response, "encode failed", http.StatusInternalServerError)
		return
	}
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.WriteHeader(status)
	_, _ = response.Write(append(body, '\n'))
}

func writeEncodedJSON(response http.ResponseWriter, status int, body []byte) {
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.WriteHeader(status)
	_, _ = response.Write(body)
}

func singleHeader(header http.Header, name string) (string, bool) {
	values := header.Values(name)
	returnValue := ""
	if len(values) == 1 {
		returnValue = values[0]
	}
	return returnValue, len(values) == 1
}

func decodedTextHeader(header http.Header, name string, maxBytes int) (string, bool) {
	encoded, ok := singleHeader(header, name)
	if !ok || encoded == "" || len(encoded) > base64.RawURLEncoding.EncodedLen(maxBytes) {
		return "", false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(decoded) > maxBytes || !utf8.Valid(decoded) {
		return "", false
	}
	return string(decoded), true
}

func exactMediaType(header http.Header, want string) bool {
	values := header.Values("Content-Type")
	if len(values) != 1 {
		return false
	}
	mediaType, parameters, err := mime.ParseMediaType(values[0])
	return err == nil && mediaType == want && len(parameters) == 0
}

func oneQuery(request *http.Request, name string) (string, bool) {
	values, exists := request.URL.Query()[name]
	returnValue := ""
	if exists && len(values) == 1 {
		returnValue = values[0]
	}
	return returnValue, exists && len(values) == 1
}

func optionalQuery(request *http.Request, name string) (string, bool) {
	values, exists := request.URL.Query()[name]
	if !exists {
		return "", false
	}
	if len(values) != 1 || values[0] == "" {
		return "", true
	}
	return values[0], true
}

func noUnexpectedQuery(request *http.Request, allowed ...string) bool {
	set := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		set[name] = struct{}{}
	}
	for name := range request.URL.Query() {
		if _, ok := set[name]; !ok {
			return false
		}
	}
	return true
}

func queryLimit(request *http.Request, defaultValue int) (int, error) {
	values, exists := request.URL.Query()["limit"]
	if !exists {
		return defaultValue, nil
	}
	if len(values) != 1 {
		return 0, errors.New("duplicate limit")
	}
	value, err := strconv.Atoi(values[0])
	if err != nil || value < 1 || value > 100 {
		return 0, errors.New("invalid limit")
	}
	return value, nil
}

func queryOffset(request *http.Request) (int, error) {
	values, exists := request.URL.Query()["offset"]
	if !exists {
		return 0, nil
	}
	if len(values) != 1 {
		return 0, errors.New("duplicate offset")
	}
	value, err := strconv.Atoi(values[0])
	if err != nil || value < 0 || value >= maxSearchWindow {
		return 0, errors.New("invalid offset")
	}
	return value, nil
}

func validIdentifier(value string) bool {
	return validOpaque(value, maxIdentifierText)
}

func validOpaque(value string, maxBytes int) bool {
	if value == "" || len(value) > maxBytes || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validDisplay(value string, maxBytes int) bool {
	if value == "" || len(value) > maxBytes || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if character == 0 || (unicode.IsControl(character) && character != '\t' && character != '\n') {
			return false
		}
	}
	return true
}

func validFilename(value string) bool {
	if !validOpaque(value, maxHeaderText) || filepath.IsAbs(value) || filepath.Base(value) != value || strings.ContainsAny(value, `/\`) {
		return false
	}
	_, err := ingest.DetectTextFormat(value)
	return err == nil
}

func validSearch(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= maxHeaderText && utf8.ValidString(value) && utf8.RuneCountInString(value) >= 3
}

func newRequestID() string {
	var random [16]byte
	if _, err := io.ReadFull(rand.Reader, random[:]); err != nil {
		return "request-id-unavailable"
	}
	return hex.EncodeToString(random[:])
}
