// Package v1 validates the checked-in, implementation-bound CORE HTTP and
// production-surface contracts. It is deliberately a build/test-time package;
// the desktop process does not parse its own OpenAPI document at runtime.
package v1

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	maxContractBytes  = 2 << 20
	maxContractRoutes = 64
	maxContractDepth  = 96
	maxContractTokens = 200000
)

var requiredForbiddenSegments = []string{
	"agent", "agents", "batch", "batches", "embedding", "embeddings",
	"evaluation", "evaluations", "kbhealth", "memories", "memory", "notification",
	"notifications", "reindex", "reindexing", "rerank", "reranker", "vector", "vectors",
}

var forbiddenLegacyBackendModuleSegments = []string{"flyway", "jdbc", "mariadb", "mysql"}

type operationShape struct {
	RequestSchema string
	RequestBinary bool
	Query         []string
	RequiredQuery []string
	ExtraHeaders  []string
	SuccessSchema map[int]string
}

// implementationShapes freezes the request/response wire shape that is not
// expressible by method/path alone. Names are component schema names; an empty
// success schema means the implementation returns no body (204 or HEAD).
var implementationShapes = map[string]operationShape{
	"addCollectionMember":      {RequestSchema: "CollectionMemberMutationRequest", SuccessSchema: map[int]string{200: "CollectionMutationResponse"}},
	"ask":                      {RequestSchema: "AskRequest", SuccessSchema: map[int]string{200: "AnswerEnvelope", 202: "AnswerEnvelope"}},
	"cancelBackup":             {RequestSchema: "BackupCancelRequest", SuccessSchema: map[int]string{202: "BackupOperationStatus"}},
	"cancelJob":                {RequestSchema: "JobCancelRequest", SuccessSchema: map[int]string{204: ""}},
	"configureOllama":          {RequestSchema: "OllamaConfigureRequest", SuccessSchema: map[int]string{200: "OllamaConfigResult"}},
	"createCollection":         {RequestSchema: "CollectionCreateRequest", SuccessSchema: map[int]string{200: "CollectionCreateResponse", 201: "CollectionCreateResponse"}},
	"createBackup":             {RequestSchema: "BackupCreateRequest", SuccessSchema: map[int]string{202: "BackupOperationStatus"}},
	"createConversation":       {RequestSchema: "ConversationCreateRequest", SuccessSchema: map[int]string{200: "ConversationCreateResponse", 201: "ConversationCreateResponse"}},
	"deleteConversation":       {RequestSchema: "ConversationDeleteRequest", SuccessSchema: map[int]string{200: "ConversationDeleteResponse"}},
	"exchangeBootstrap":        {ExtraHeaders: []string{"X-MindWeaver-Bootstrap"}, SuccessSchema: map[int]string{200: "CSRFTokenResponse"}},
	"getAnswer":                {Query: []string{"id"}, RequiredQuery: []string{"id"}, SuccessSchema: map[int]string{200: "AnswerEnvelope", 202: "AnswerEnvelope"}},
	"getBackupStatus":          {Query: []string{"operationId"}, RequiredQuery: []string{"operationId"}, SuccessSchema: map[int]string{200: "BackupOperationStatus"}},
	"getDiagnostics":           {SuccessSchema: map[int]string{200: "Diagnostics"}},
	"getHealth":                {SuccessSchema: map[int]string{200: "Health"}},
	"getJob":                   {Query: []string{"id"}, RequiredQuery: []string{"id"}, SuccessSchema: map[int]string{200: "JobEnvelope"}},
	"getOllamaConfiguration":   {SuccessSchema: map[int]string{200: "OllamaConfigResponse"}},
	"getPurgeStatus":           {Query: []string{"id"}, RequiredQuery: []string{"id"}, SuccessSchema: map[int]string{200: "PurgeStatus"}},
	"getRuntime":               {SuccessSchema: map[int]string{200: "Runtime"}},
	"headHealth":               {SuccessSchema: map[int]string{200: ""}},
	"listCollectionMembers":    {Query: []string{"collection_id", "cursor", "limit"}, RequiredQuery: []string{"collection_id"}, SuccessSchema: map[int]string{200: "CollectionMembersPage"}},
	"listCollections":          {Query: []string{"cursor", "limit"}, SuccessSchema: map[int]string{200: "CollectionsPage"}},
	"listConversationMessages": {Query: []string{"conversation_id", "cursor", "limit"}, RequiredQuery: []string{"conversation_id"}, SuccessSchema: map[int]string{200: "MessagesPage"}},
	"listConversations":        {Query: []string{"cursor", "limit"}, SuccessSchema: map[int]string{200: "ConversationsPage"}},
	"listDocuments":            {Query: []string{"cursor", "limit"}, SuccessSchema: map[int]string{200: "DocumentsPage"}},
	"listPurges":               {Query: []string{"cursor", "limit"}, SuccessSchema: map[int]string{200: "PurgesPage"}},
	"probeOllama":              {RequestSchema: "OllamaProbeRequest", SuccessSchema: map[int]string{200: "OllamaProbeResponse"}},
	"purgeDocument":            {RequestSchema: "DocumentMutationRequest", SuccessSchema: map[int]string{200: "PurgeResult", 202: "PurgeResult"}},
	"refreshCSRF":              {SuccessSchema: map[int]string{200: "CSRFTokenResponse"}},
	"removeCollectionMember":   {RequestSchema: "CollectionMemberMutationRequest", SuccessSchema: map[int]string{200: "CollectionMutationResponse"}},
	"restoreDocument":          {RequestSchema: "DocumentMutationRequest", SuccessSchema: map[int]string{200: "LifecycleEnvelope"}},
	"retryDocumentIngestion":   {RequestSchema: "DocumentMutationRequest", SuccessSchema: map[int]string{200: "DocumentRetryResponse"}},
	"search":                   {Query: []string{"collection_id", "limit", "offset", "q"}, RequiredQuery: []string{"q"}, SuccessSchema: map[int]string{200: "SearchPage"}},
	"trashDocument":            {RequestSchema: "DocumentMutationRequest", SuccessSchema: map[int]string{200: "LifecycleEnvelope"}},
	"uploadDocument":           {RequestBinary: true, ExtraHeaders: []string{"X-MindWeaver-Filename-B64", "X-MindWeaver-Title-B64"}, SuccessSchema: map[int]string{200: "UploadResponse", 202: "UploadResponse"}},
}

//go:embed openapi.json
var openAPIDocument []byte

//go:embed core-surface.v1.json
var surfaceDocument []byte

// Documents returns defensive copies of the two canonical contract files.
func Documents() (openAPI, surface []byte) {
	return append([]byte(nil), openAPIDocument...), append([]byte(nil), surfaceDocument...)
}

// Snapshot is the small validated projection consumed by implementation
// contract tests and release tooling. It intentionally does not expose a
// general OpenAPI parser.
type Snapshot struct {
	Routes                []SurfaceRoute
	ProblemStatusByCode   map[string]int
	ProblemRecoveryByCode map[string]ProblemRecovery
	Surface               Surface
}

// ProblemRecovery is the stable, content-free client decision paired with a
// Problem code. Retryable never overrides an operation's idempotency or CAS
// contract.
type ProblemRecovery struct {
	Retryable  bool   `json:"retryable"`
	UserAction string `json:"userAction"`
}

var requiredProblemRecovery = map[string]ProblemRecovery{
	"CONFLICT":            {UserAction: "refresh_state"},
	"FORBIDDEN":           {UserAction: "restart_session"},
	"INTERNAL":            {UserAction: "inspect_diagnostics"},
	"INVALID_ARGUMENT":    {UserAction: "correct_request"},
	"NOT_FOUND":           {UserAction: "refresh_state"},
	"RESOURCE_LIMIT":      {UserAction: "review_limits"},
	"SERVICE_UNAVAILABLE": {Retryable: true, UserAction: "retry_later"},
	"UNAUTHENTICATED":     {UserAction: "restart_session"},
}

// ValidateEmbedded validates and cross-binds the checked-in documents.
func ValidateEmbedded() (Snapshot, error) {
	return Validate(openAPIDocument, surfaceDocument)
}

// Validate rejects non-canonical JSON, duplicate/unknown fields, unsupported
// methods, unbounded paths, duplicate operations, unresolved references, and
// any drift between OpenAPI operations and the versioned CORE surface.
func Validate(openAPIRaw, surfaceRaw []byte) (Snapshot, error) {
	var document openAPIDocumentWire
	if err := decodeCanonicalStrict(openAPIRaw, &document); err != nil {
		return Snapshot{}, fmt.Errorf("openapi: %w", err)
	}
	var surface Surface
	if err := decodeCanonicalStrict(surfaceRaw, &surface); err != nil {
		return Snapshot{}, fmt.Errorf("surface: %w", err)
	}
	if err := validateSurface(surface); err != nil {
		return Snapshot{}, fmt.Errorf("surface: %w", err)
	}
	problemMap, err := validateOpenAPI(document, surface)
	if err != nil {
		return Snapshot{}, fmt.Errorf("openapi: %w", err)
	}
	return Snapshot{
		Routes:                append([]SurfaceRoute(nil), surface.Routes...),
		ProblemStatusByCode:   problemMap,
		ProblemRecoveryByCode: cloneRecoveryMap(document.Components.Schemas["Problem"].RecoveryByCode),
		Surface:               surface,
	}, nil
}

type openAPIDocumentWire struct {
	OpenAPI           string              `json:"openapi"`
	Info              infoWire            `json:"info"`
	JSONSchemaDialect string              `json:"jsonSchemaDialect"`
	Servers           []serverWire        `json:"servers"`
	Paths             map[string]pathWire `json:"paths"`
	Components        componentsWire      `json:"components"`
	Auth              authWire            `json:"x-mindweaver-auth"`
	ETag              string              `json:"x-mindweaver-etag"`
}

type infoWire struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

type serverWire struct {
	URL         string                    `json:"url"`
	Description string                    `json:"description"`
	Variables   map[string]serverVariable `json:"variables"`
}

type serverVariable struct {
	Default     string `json:"default"`
	Description string `json:"description"`
}

type authWire struct {
	BootstrapHeader       string `json:"bootstrapHeader"`
	BootstrapPath         string `json:"bootstrapPath"`
	CSRFHeader            string `json:"csrfHeader"`
	CSRFRefreshPath       string `json:"csrfRefreshPath"`
	SessionCookie         string `json:"sessionCookie"`
	SessionPersistence    string `json:"sessionPersistence"`
	SameOriginEnforcement string `json:"sameOriginEnforcement"`
}

type pathWire struct {
	Get    *operationWire `json:"get,omitempty"`
	Head   *operationWire `json:"head,omitempty"`
	Post   *operationWire `json:"post,omitempty"`
	Put    *operationWire `json:"put,omitempty"`
	Delete *operationWire `json:"delete,omitempty"`
}

type operationWire struct {
	OperationID       string                  `json:"operationId"`
	Summary           string                  `json:"summary"`
	Description       string                  `json:"description,omitempty"`
	Parameters        []parameterWire         `json:"parameters,omitempty"`
	RequestBody       *requestBodyWire        `json:"requestBody,omitempty"`
	Responses         map[string]responseWire `json:"responses"`
	Session           string                  `json:"x-mindweaver-session"`
	CSRF              string                  `json:"x-mindweaver-csrf"`
	SameOriginFetch   string                  `json:"x-mindweaver-same-origin-fetch,omitempty"`
	RetrySafety       string                  `json:"x-mindweaver-retry-safety,omitempty"`
	OptimisticControl string                  `json:"x-mindweaver-optimistic-control,omitempty"`
}

type parameterWire struct {
	Ref         string      `json:"$ref,omitempty"`
	Name        string      `json:"name,omitempty"`
	In          string      `json:"in,omitempty"`
	Description string      `json:"description,omitempty"`
	Required    bool        `json:"required,omitempty"`
	Schema      *schemaWire `json:"schema,omitempty"`
}

type requestBodyWire struct {
	Description string               `json:"description,omitempty"`
	Required    bool                 `json:"required"`
	Content     map[string]mediaWire `json:"content"`
}

type responseWire struct {
	Ref         string                `json:"$ref,omitempty"`
	Description string                `json:"description,omitempty"`
	Headers     map[string]headerWire `json:"headers,omitempty"`
	Content     map[string]mediaWire  `json:"content,omitempty"`
}

type headerWire struct {
	Description string     `json:"description"`
	Schema      schemaWire `json:"schema"`
}

type mediaWire struct {
	Schema schemaWire `json:"schema"`
}

type componentsWire struct {
	Parameters map[string]parameterWire `json:"parameters"`
	Responses  map[string]responseWire  `json:"responses"`
	Schemas    map[string]schemaWire    `json:"schemas"`
}

// schemaWire is the intentionally small JSON Schema subset used by this CORE
// document. Unknown schema vocabulary fails closed instead of being silently
// accepted by a permissive map decoder.
type schemaWire struct {
	Ref                  string                     `json:"$ref,omitempty"`
	Type                 string                     `json:"type,omitempty"`
	Format               string                     `json:"format,omitempty"`
	Description          string                     `json:"description,omitempty"`
	Properties           map[string]schemaWire      `json:"properties,omitempty"`
	Required             []string                   `json:"required,omitempty"`
	AdditionalProperties *bool                      `json:"additionalProperties,omitempty"`
	Items                *schemaWire                `json:"items,omitempty"`
	Enum                 []string                   `json:"enum,omitempty"`
	OneOf                []schemaWire               `json:"oneOf,omitempty"`
	Minimum              *int64                     `json:"minimum,omitempty"`
	Maximum              *int64                     `json:"maximum,omitempty"`
	MinLength            *int                       `json:"minLength,omitempty"`
	MaxLength            *int                       `json:"maxLength,omitempty"`
	Pattern              string                     `json:"pattern,omitempty"`
	MinItems             *int                       `json:"minItems,omitempty"`
	MaxItems             *int                       `json:"maxItems,omitempty"`
	StatusByCode         map[string]int             `json:"x-mindweaver-status-by-code,omitempty"`
	RecoveryByCode       map[string]ProblemRecovery `json:"x-mindweaver-recovery-by-code,omitempty"`
}

// Surface is the versioned allowlist for shipped CORE packages, routes,
// migrations, final SQLite tables, and executable command surfaces.
type Surface struct {
	SchemaVersion       string              `json:"schemaVersion"`
	Module              string              `json:"module"`
	ProductionDiscovery productionDiscovery `json:"productionDiscovery"`
	Packages            []string            `json:"packages"`
	Routes              []SurfaceRoute      `json:"routes"`
	Migrations          []string            `json:"migrations"`
	Tables              []string            `json:"tables"`
	Commands            []SurfaceCommand    `json:"commands"`
	ForbiddenSegments   []string            `json:"forbiddenSegments"`
}

type productionDiscovery struct {
	EvidenceOnlyMainPackages []string `json:"evidenceOnlyMainPackages"`
	ExternalModules          []string `json:"externalModules"`
	LibraryRoots             []string `json:"libraryRoots"`
	ExcludedRootPrefixes     []string `json:"excludedRootPrefixes"`
	ExcludedFileSuffixes     []string `json:"excludedFileSuffixes"`
}

// SurfaceRoute binds one real HTTP operation to its externally visible
// concurrency, idempotency, session, and Problem status behavior.
type SurfaceRoute struct {
	Method            string `json:"method"`
	Path              string `json:"path"`
	OperationID       string `json:"operationId"`
	Session           string `json:"session"`
	CSRF              string `json:"csrf"`
	IdempotencyKey    string `json:"idempotencyKey"`
	SuccessStatuses   []int  `json:"successStatuses"`
	ProblemStatuses   []int  `json:"problemStatuses"`
	TransportStatuses []int  `json:"transportStatuses"`
}

// SurfaceCommand distinguishes the user CLI from the state-free internal PDF
// protocol helper. Verbs and nested verbs are exact, not documentation hints.
type SurfaceCommand struct {
	Name                 string              `json:"name"`
	Public               bool                `json:"public"`
	SourceManifestSHA256 string              `json:"sourceManifestSHA256"`
	Packages             []string            `json:"packages"`
	ExternalModules      []string            `json:"externalModules"`
	DefaultVerb          string              `json:"defaultVerb,omitempty"`
	Verbs                []string            `json:"verbs"`
	Aliases              []string            `json:"aliases"`
	NestedVerbs          map[string][]string `json:"nestedVerbs"`
}

func validateOpenAPI(document openAPIDocumentWire, surface Surface) (map[string]int, error) {
	if document.OpenAPI != "3.1.0" || document.JSONSchemaDialect != "https://json-schema.org/draft/2020-12/schema" {
		return nil, errors.New("version or JSON Schema dialect is not frozen")
	}
	if document.Info.Title != "MindWeaver CORE local API" || document.Info.Version != "1.0.0" || strings.TrimSpace(document.Info.Description) == "" {
		return nil, errors.New("info is incomplete")
	}
	if len(document.Servers) != 1 || document.Servers[0].URL != "http://127.0.0.1:{port}" || document.Servers[0].Variables["port"].Default != "0" {
		return nil, errors.New("server must be the process-selected IPv4 loopback port")
	}
	if document.ETag != "unsupported; use expectedRevision or expectedVersion in strict JSON bodies" {
		return nil, errors.New("ETag policy is not the implemented policy")
	}
	if document.Auth.BootstrapHeader != "X-MindWeaver-Bootstrap" || document.Auth.BootstrapPath != "/bootstrap/exchange" ||
		document.Auth.CSRFHeader != "X-MindWeaver-CSRF" || document.Auth.CSRFRefreshPath != "/session/csrf" ||
		document.Auth.SessionCookie != "mindweaver_session_<32 lowercase hex>; HttpOnly; SameSite=Strict; Path=/" ||
		document.Auth.SessionPersistence != "single process-memory session; invalidated on shutdown" ||
		document.Auth.SameOriginEnforcement != "exact Host/Origin/127.0.0.1 peer plus browser Fetch Metadata" {
		return nil, errors.New("browser session contract drift")
	}
	if err := validateBoundaryParameters(document.Components); err != nil {
		return nil, err
	}
	if err := rejectUnsupportedETag(document); err != nil {
		return nil, err
	}
	problem, err := validateProblemSchemas(document.Components)
	if err != nil {
		return nil, err
	}
	if err := validateProblemComponents(document.Components, problem.StatusByCode); err != nil {
		return nil, err
	}
	if err := validateReferences(document); err != nil {
		return nil, err
	}
	if err := validateCompatibilityShapes(document.Components); err != nil {
		return nil, err
	}
	if err := validateRAGResponseSchemas(document.Components); err != nil {
		return nil, err
	}

	expected := make(map[string]SurfaceRoute, len(surface.Routes))
	for _, route := range surface.Routes {
		expected[route.Method+" "+route.Path] = route
	}
	seenOperations := make(map[string]string)
	seenRoutes := make(map[string]struct{})
	for routePath, item := range document.Paths {
		if err := validPath(routePath); err != nil {
			return nil, err
		}
		operations := []struct {
			method    string
			operation *operationWire
		}{
			{http.MethodGet, item.Get}, {http.MethodHead, item.Head}, {http.MethodPost, item.Post},
			{http.MethodPut, item.Put}, {http.MethodDelete, item.Delete},
		}
		for _, candidate := range operations {
			if candidate.operation == nil {
				continue
			}
			key := candidate.method + " " + routePath
			want, exists := expected[key]
			if !exists {
				return nil, fmt.Errorf("unimplemented operation %s", key)
			}
			if previous, duplicate := seenOperations[candidate.operation.OperationID]; duplicate {
				return nil, fmt.Errorf("duplicate operationId %q at %s and %s", candidate.operation.OperationID, previous, key)
			}
			seenOperations[candidate.operation.OperationID] = key
			seenRoutes[key] = struct{}{}
			if err := validateOperation(*candidate.operation, want, document.Components); err != nil {
				return nil, fmt.Errorf("%s: %w", key, err)
			}
		}
	}
	if len(seenRoutes) != len(expected) {
		missing := make([]string, 0)
		for key := range expected {
			if _, ok := seenRoutes[key]; !ok {
				missing = append(missing, key)
			}
		}
		sort.Strings(missing)
		return nil, fmt.Errorf("missing implemented operations: %s", strings.Join(missing, ", "))
	}
	if len(implementationShapes) != len(seenOperations) {
		return nil, errors.New("implementation request/response shape count drift")
	}
	return cloneStatusMap(problem.StatusByCode), nil
}

func validateOperation(operation operationWire, want SurfaceRoute, components componentsWire) error {
	if operation.OperationID != want.OperationID || len(operation.OperationID) == 0 || len(operation.OperationID) > 64 || !validOperationID(operation.OperationID) {
		return errors.New("operationId drift or invalid operationId")
	}
	if strings.TrimSpace(operation.Summary) == "" || operation.Session != want.Session || operation.CSRF != want.CSRF {
		return errors.New("summary/session/CSRF contract drift")
	}
	wantIdempotency := want.IdempotencyKey == "required"
	hasIdempotency, hasCSRF := false, false
	for _, parameter := range operation.Parameters {
		switch parameter.Ref {
		case "#/components/parameters/IdempotencyKey":
			hasIdempotency = true
		case "#/components/parameters/CSRFToken":
			hasCSRF = true
		}
	}
	if hasIdempotency != wantIdempotency || hasCSRF != (want.CSRF == "required") {
		return errors.New("idempotency or CSRF header contract drift")
	}
	shape, ok := implementationShapes[operation.OperationID]
	if !ok {
		return errors.New("request/response shape is not implementation-bound")
	}
	if err := validateParameters(operation.Parameters, want, shape, components); err != nil {
		return err
	}
	if err := validateRequestBody(operation.RequestBody, shape); err != nil {
		return err
	}
	actualSuccess, actualProblem, actualTransport := []int{}, []int{}, []int{}
	for rawStatus, response := range operation.Responses {
		status, err := strconv.Atoi(rawStatus)
		if err != nil || status < 100 || status > 599 {
			return fmt.Errorf("invalid response status %q", rawStatus)
		}
		switch {
		case status >= 200 && status < 300:
			actualSuccess = append(actualSuccess, status)
		case strings.HasPrefix(response.Ref, "#/components/responses/Problem"):
			if response.Ref != "#/components/responses/Problem"+strconv.Itoa(status) {
				return fmt.Errorf("status %d references the wrong Problem response", status)
			}
			actualProblem = append(actualProblem, status)
		case strings.HasPrefix(response.Ref, "#/components/responses/Rejected"):
			if response.Ref != "#/components/responses/Rejected"+strconv.Itoa(status) {
				return fmt.Errorf("status %d references the wrong rejected transport response", status)
			}
			actualTransport = append(actualTransport, status)
		default:
			return fmt.Errorf("error status %d is not bound to Problem or rejected transport response", status)
		}
		_, hasRetryAfter := response.Headers["Retry-After"]
		allowedRetryAfter := status == http.StatusAccepted &&
			((want.Method == http.MethodGet && want.Path == "/api/v1/answers") ||
				(want.Method == http.MethodPost && want.Path == "/api/v1/ask"))
		if hasRetryAfter != allowedRetryAfter {
			return errors.New("Retry-After header contract drift")
		}
	}
	if !sameInts(actualSuccess, want.SuccessStatuses) || !sameInts(actualProblem, want.ProblemStatuses) || !sameInts(actualTransport, want.TransportStatuses) {
		return errors.New("response status contract drift")
	}
	if err := validateSuccessBodies(operation.Responses, shape); err != nil {
		return err
	}
	return nil
}

func validateParameters(parameters []parameterWire, route SurfaceRoute, shape operationShape, components componentsWire) error {
	query, requiredQuery, headers := []string{}, []string{}, []string{}
	seen := map[string]struct{}{}
	for _, parameter := range parameters {
		resolved := parameter
		if parameter.Ref != "" {
			const prefix = "#/components/parameters/"
			if !strings.HasPrefix(parameter.Ref, prefix) {
				return errors.New("operation parameter uses an external or non-parameter reference")
			}
			name := strings.TrimPrefix(parameter.Ref, prefix)
			var exists bool
			resolved, exists = components.Parameters[name]
			if !exists {
				return errors.New("operation parameter reference is unresolved")
			}
		}
		if resolved.Name == "" || resolved.Schema == nil || (resolved.In != "query" && resolved.In != "header") {
			return errors.New("operation parameter shape is invalid")
		}
		key := resolved.In + "\x00" + strings.ToLower(resolved.Name)
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("duplicate parameter %s %s", resolved.In, resolved.Name)
		}
		seen[key] = struct{}{}
		switch resolved.In {
		case "query":
			query = append(query, resolved.Name)
			if resolved.Required {
				requiredQuery = append(requiredQuery, resolved.Name)
			}
		case "header":
			if !resolved.Required {
				return errors.New("all implemented request header parameters are required")
			}
			headers = append(headers, resolved.Name)
		}
	}
	wantHeaders := append([]string(nil), shape.ExtraHeaders...)
	if route.CSRF == "required" {
		wantHeaders = append(wantHeaders, "X-MindWeaver-CSRF")
	}
	if route.IdempotencyKey == "required" {
		wantHeaders = append(wantHeaders, "Idempotency-Key")
	}
	sort.Strings(query)
	sort.Strings(requiredQuery)
	sort.Strings(headers)
	sort.Strings(wantHeaders)
	if !equalStrings(query, shape.Query) || !equalStrings(requiredQuery, shape.RequiredQuery) || !equalStrings(headers, wantHeaders) {
		return errors.New("query or request header parameter contract drift")
	}
	return nil
}

func validateRequestBody(body *requestBodyWire, shape operationShape) error {
	switch {
	case shape.RequestSchema == "" && !shape.RequestBinary:
		if body != nil {
			return errors.New("operation invents an unimplemented request body")
		}
		return nil
	case body == nil || !body.Required || len(body.Content) != 1:
		return errors.New("implemented request body is missing or not required")
	case shape.RequestBinary:
		media, ok := body.Content["application/octet-stream"]
		if !ok || media.Schema.Type != "string" || media.Schema.Format != "binary" {
			return errors.New("binary request body contract drift")
		}
		return nil
	default:
		media, ok := body.Content["application/json"]
		if !ok || media.Schema.Ref != "#/components/schemas/"+shape.RequestSchema {
			return errors.New("strict JSON request schema contract drift")
		}
		return nil
	}
}

func validateSuccessBodies(responses map[string]responseWire, shape operationShape) error {
	if len(shape.SuccessSchema) == 0 {
		return errors.New("implementation success response shape is empty")
	}
	for status, schemaName := range shape.SuccessSchema {
		response, ok := responses[strconv.Itoa(status)]
		if !ok {
			return fmt.Errorf("missing success response %d", status)
		}
		if schemaName == "" {
			if len(response.Content) != 0 {
				return fmt.Errorf("status %d must not have a response body", status)
			}
			continue
		}
		if len(response.Content) != 1 {
			return fmt.Errorf("status %d must have one JSON response media type", status)
		}
		media, ok := response.Content["application/json"]
		if !ok || media.Schema.Ref != "#/components/schemas/"+schemaName {
			return fmt.Errorf("status %d response schema drift", status)
		}
	}
	return nil
}

func validateProblemSchemas(components componentsWire) (schemaWire, error) {
	problem, ok := components.Schemas["Problem"]
	if !ok || problem.Type != "object" || problem.AdditionalProperties == nil || !*problem.AdditionalProperties ||
		len(problem.StatusByCode) != 8 || len(problem.RecoveryByCode) != 8 {
		return schemaWire{}, errors.New("Problem schema or status mapping is missing")
	}
	propertyNames := sortedMapKeys(problem.Properties)
	if !equalStrings(propertyNames, []string{"code", "detail", "instance", "requestId", "retryable", "status", "title", "type", "userAction"}) ||
		!equalStrings(problem.Required, []string{"code", "requestId", "retryable", "status", "title", "type", "userAction"}) {
		return schemaWire{}, errors.New("Problem property or required-field contract drift")
	}
	codeNames := sortedMapKeys(problem.StatusByCode)
	if !equalStrings(codeNames, sortedMapKeys(problem.RecoveryByCode)) ||
		!reflect.DeepEqual(problem.RecoveryByCode, requiredProblemRecovery) ||
		!equalStrings(problem.Properties["code"].Enum, codeNames) || problem.Properties["code"].Type != "string" ||
		problem.Properties["detail"].Type != "string" || problem.Properties["instance"].Type != "string" ||
		problem.Properties["requestId"].Type != "string" || problem.Properties["requestId"].MinLength == nil || *problem.Properties["requestId"].MinLength != 1 ||
		problem.Properties["retryable"].Type != "boolean" ||
		problem.Properties["status"].Type != "integer" || problem.Properties["status"].Minimum == nil || *problem.Properties["status"].Minimum != 400 ||
		problem.Properties["status"].Maximum == nil || *problem.Properties["status"].Maximum != 599 ||
		problem.Properties["title"].Type != "string" || problem.Properties["title"].MinLength == nil || *problem.Properties["title"].MinLength != 1 ||
		problem.Properties["type"].Type != "string" || problem.Properties["type"].Format != "uri" ||
		problem.Properties["userAction"].Type != "string" || !equalStrings(problem.Properties["userAction"].Enum,
		[]string{"correct_request", "inspect_diagnostics", "refresh_state", "restart_session", "retry_later", "review_limits"}) {
		return schemaWire{}, errors.New("Problem field shape drift")
	}
	for _, recovery := range problem.RecoveryByCode {
		if !containsString(problem.Properties["userAction"].Enum, recovery.UserAction) {
			return schemaWire{}, errors.New("Problem recovery mapping uses an unknown user action")
		}
	}

	rejected, ok := components.Schemas["RejectedRequest"]
	if !ok || rejected.Type != "object" || rejected.AdditionalProperties == nil || *rejected.AdditionalProperties ||
		!equalStrings(sortedMapKeys(rejected.Properties), []string{"error"}) || !equalStrings(rejected.Required, []string{"error"}) ||
		rejected.Properties["error"].Type != "string" || !equalStrings(rejected.Properties["error"].Enum, []string{"request rejected"}) {
		return schemaWire{}, errors.New("content-free rejected request schema drift")
	}
	return problem, nil
}

func validateCompatibilityShapes(components componentsWire) error {
	requests := make(map[string]struct{})
	for _, shape := range implementationShapes {
		if shape.RequestSchema != "" {
			requests[shape.RequestSchema] = struct{}{}
		}
	}
	for name := range requests {
		schema, ok := components.Schemas[name]
		if !ok || schema.Type != "object" || schema.AdditionalProperties == nil || *schema.AdditionalProperties {
			return fmt.Errorf("request schema %s is not closed", name)
		}
	}
	seen := make(map[string]struct{})
	for _, shape := range implementationShapes {
		for _, name := range shape.SuccessSchema {
			if name != "" {
				if err := validateOpenResponseSchema(name, components, seen); err != nil {
					return err
				}
			}
		}
	}
	return validateOpenResponseSchema("Problem", components, seen)
}

func validateOpenResponseSchema(name string, components componentsWire, seen map[string]struct{}) error {
	if _, exists := seen[name]; exists {
		return nil
	}
	seen[name] = struct{}{}
	schema, ok := components.Schemas[name]
	if !ok {
		return fmt.Errorf("response schema %s is missing", name)
	}
	if schema.Type == "object" && (schema.AdditionalProperties == nil || !*schema.AdditionalProperties) {
		return fmt.Errorf("response schema %s is not additive-compatible", name)
	}
	return walkResponseSchema(schema, components, seen)
}

func walkResponseSchema(schema schemaWire, components componentsWire, seen map[string]struct{}) error {
	if schema.Type == "object" && schema.AdditionalProperties != nil && !*schema.AdditionalProperties {
		return errors.New("inline response object is not additive-compatible")
	}
	if schema.Ref != "" {
		const prefix = "#/components/schemas/"
		if strings.HasPrefix(schema.Ref, prefix) {
			return validateOpenResponseSchema(strings.TrimPrefix(schema.Ref, prefix), components, seen)
		}
	}
	for _, property := range schema.Properties {
		if err := walkResponseSchema(property, components, seen); err != nil {
			return err
		}
	}
	if schema.Items != nil {
		if err := walkResponseSchema(*schema.Items, components, seen); err != nil {
			return err
		}
	}
	for _, alternative := range schema.OneOf {
		if err := walkResponseSchema(alternative, components, seen); err != nil {
			return err
		}
	}
	return nil
}

func validateRAGResponseSchemas(components componentsWire) error {
	answer, answerOK := components.Schemas["Answer"]
	message, messageOK := components.Schemas["Message"]
	if !answerOK || !messageOK {
		return errors.New("Answer or Message schema is missing")
	}
	dateTime := schemaWire{Type: "string", Format: "date-time"}
	nullValue := schemaWire{Type: "null"}
	empty := schemaWire{Type: "string", Enum: []string{""}}
	nonEmpty := schemaWire{Type: "string", MinLength: intPointer(1)}
	code := schemaWire{Type: "string", MinLength: intPointer(1), MaxLength: intPointer(64), Pattern: "^[A-Z0-9_]+$"}
	emptyList := schemaWire{Type: "array", MaxItems: intPointer(0)}
	nonEmptyList := schemaWire{Type: "array", MinItems: intPointer(1)}
	status := func(value string) schemaWire { return schemaWire{Type: "string", Enum: []string{value}} }
	role := func(value string) schemaWire { return schemaWire{Type: "string", Enum: []string{value}} }
	version := func(minimum, maximum *int64) schemaWire {
		return schemaWire{Type: "integer", Minimum: minimum, Maximum: maximum}
	}
	object := func(properties map[string]schemaWire) schemaWire {
		return schemaWire{Type: "object", Properties: properties}
	}

	wantAnswerStates := []schemaWire{
		object(map[string]schemaWire{"citations": emptyList, "completedAt": nullValue, "content": empty, "errorCode": empty, "limitationCode": empty, "status": status("pending")}),
		object(map[string]schemaWire{"citations": nonEmptyList, "completedAt": dateTime, "content": nonEmpty, "errorCode": empty, "limitationCode": empty, "sources": nonEmptyList, "status": status("completed")}),
		object(map[string]schemaWire{"citations": emptyList, "completedAt": dateTime, "content": nonEmpty, "errorCode": empty, "limitationCode": code, "status": status("refused")}),
		object(map[string]schemaWire{"citations": emptyList, "completedAt": dateTime, "content": nonEmpty, "errorCode": code, "limitationCode": code, "status": status("failed")}),
	}
	if answer.Type != "object" || !reflect.DeepEqual(answer.OneOf, wantAnswerStates) ||
		!equalStrings(answer.Required, []string{"citations", "completedAt", "content", "conversationId", "conversationRevision", "createdAt", "errorCode", "id", "limitationCode", "providerConfigVersion", "question", "reconcileAfter", "scopeCollectionId", "sources", "status"}) ||
		!reflect.DeepEqual(answer.Properties["reconcileAfter"], dateTime) ||
		!reflect.DeepEqual(answer.Properties["status"].Enum, []string{"completed", "failed", "pending", "refused"}) {
		return errors.New("Answer state or recovery schema drift")
	}

	zero, one := int64(0), int64(1)
	wantMessageStates := []schemaWire{
		object(map[string]schemaWire{"citations": emptyList, "completedAt": dateTime, "content": nonEmpty, "errorCode": empty, "limitationCode": empty, "providerConfigVersion": version(&zero, &zero), "reconcileAfter": nullValue, "role": role("user"), "sources": emptyList, "status": status("completed")}),
		object(map[string]schemaWire{"citations": emptyList, "completedAt": nullValue, "content": empty, "errorCode": empty, "limitationCode": empty, "providerConfigVersion": version(&one, nil), "reconcileAfter": dateTime, "role": role("assistant"), "status": status("pending")}),
		object(map[string]schemaWire{"citations": nonEmptyList, "completedAt": dateTime, "content": nonEmpty, "errorCode": empty, "limitationCode": empty, "providerConfigVersion": version(&one, nil), "reconcileAfter": dateTime, "role": role("assistant"), "sources": nonEmptyList, "status": status("completed")}),
		object(map[string]schemaWire{"citations": emptyList, "completedAt": dateTime, "content": nonEmpty, "errorCode": empty, "limitationCode": code, "providerConfigVersion": version(&one, nil), "reconcileAfter": dateTime, "role": role("assistant"), "status": status("refused")}),
		object(map[string]schemaWire{"citations": emptyList, "completedAt": dateTime, "content": nonEmpty, "errorCode": code, "limitationCode": code, "providerConfigVersion": version(&one, nil), "reconcileAfter": dateTime, "role": role("assistant"), "status": status("failed")}),
	}
	if message.Type != "object" || !reflect.DeepEqual(message.OneOf, wantMessageStates) ||
		!equalStrings(message.Required, []string{"citations", "completedAt", "content", "conversationId", "createdAt", "errorCode", "id", "limitationCode", "ordinal", "providerConfigVersion", "reconcileAfter", "role", "scopeCollectionId", "sources", "status"}) ||
		!reflect.DeepEqual(message.Properties["role"].Enum, []string{"assistant", "user"}) ||
		!reflect.DeepEqual(message.Properties["status"].Enum, []string{"completed", "failed", "pending", "refused"}) ||
		!reflect.DeepEqual(message.Properties["reconcileAfter"].OneOf, []schemaWire{dateTime, nullValue}) {
		return errors.New("Message state or recovery schema drift")
	}
	return nil
}

func intPointer(value int) *int { return &value }

func cloneRecoveryMap(source map[string]ProblemRecovery) map[string]ProblemRecovery {
	result := make(map[string]ProblemRecovery, len(source))
	for code, recovery := range source {
		result[code] = recovery
	}
	return result
}

func containsString(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func validateProblemComponents(components componentsWire, mapping map[string]int) error {
	for code, status := range mapping {
		if code == "" || status < 400 || status > 599 {
			return errors.New("invalid Problem status mapping")
		}
		name := "Problem" + strconv.Itoa(status)
		response, ok := components.Responses[name]
		if !ok || response.Ref != "" || response.Description != code {
			return fmt.Errorf("missing response component %s", name)
		}
		media, ok := response.Content["application/problem+json"]
		if !ok || media.Schema.Ref != "#/components/schemas/Problem" {
			return fmt.Errorf("%s does not use Problem", name)
		}
	}
	return nil
}

func validateBoundaryParameters(components componentsWire) error {
	idempotency := components.Parameters["IdempotencyKey"]
	csrf := components.Parameters["CSRFToken"]
	bootstrap := components.Parameters["BootstrapToken"]
	if idempotency.Name != "Idempotency-Key" || idempotency.In != "header" || !idempotency.Required || idempotency.Schema == nil {
		return errors.New("Idempotency-Key parameter drift")
	}
	if csrf.Name != "X-MindWeaver-CSRF" || csrf.In != "header" || !csrf.Required || csrf.Schema == nil ||
		bootstrap.Name != "X-MindWeaver-Bootstrap" || bootstrap.In != "header" || !bootstrap.Required || bootstrap.Schema == nil {
		return errors.New("bootstrap or CSRF parameter drift")
	}
	return nil
}

func rejectUnsupportedETag(document openAPIDocumentWire) error {
	checkParameters := func(parameters []parameterWire) error {
		for _, parameter := range parameters {
			if strings.EqualFold(parameter.Name, "If-Match") || strings.EqualFold(parameter.Name, "ETag") {
				return errors.New("ETag and If-Match are not implemented")
			}
		}
		return nil
	}
	componentParameters := make([]parameterWire, 0, len(document.Components.Parameters))
	for _, parameter := range document.Components.Parameters {
		componentParameters = append(componentParameters, parameter)
	}
	if err := checkParameters(componentParameters); err != nil {
		return err
	}
	for _, response := range document.Components.Responses {
		for name := range response.Headers {
			if strings.EqualFold(name, "ETag") {
				return errors.New("ETag response header is not implemented")
			}
		}
	}
	for _, item := range document.Paths {
		operations := []*operationWire{item.Get, item.Head, item.Post, item.Put, item.Delete}
		for _, operation := range operations {
			if operation == nil {
				continue
			}
			if err := checkParameters(operation.Parameters); err != nil {
				return err
			}
			for _, response := range operation.Responses {
				for name := range response.Headers {
					if strings.EqualFold(name, "ETag") {
						return errors.New("ETag response header is not implemented")
					}
				}
			}
		}
	}
	return nil
}

func validateReferences(document openAPIDocumentWire) error {
	encoded, err := json.Marshal(document)
	if err != nil {
		return err
	}
	var value any
	if err := json.Unmarshal(encoded, &value); err != nil {
		return err
	}
	return walkReferences(value, document.Components)
}

func walkReferences(value any, components componentsWire) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "$ref" {
				ref, ok := child.(string)
				if !ok || !referenceExists(ref, components) {
					return fmt.Errorf("unresolved or external reference %v", child)
				}
			}
			if err := walkReferences(child, components); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := walkReferences(child, components); err != nil {
				return err
			}
		}
	}
	return nil
}

func referenceExists(ref string, components componentsWire) bool {
	const prefix = "#/components/"
	if !strings.HasPrefix(ref, prefix) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(ref, prefix), "/")
	if len(parts) != 2 || parts[1] == "" {
		return false
	}
	switch parts[0] {
	case "schemas":
		_, ok := components.Schemas[parts[1]]
		return ok
	case "responses":
		_, ok := components.Responses[parts[1]]
		return ok
	case "parameters":
		_, ok := components.Parameters[parts[1]]
		return ok
	default:
		return false
	}
}

func validateSurface(surface Surface) error {
	if surface.SchemaVersion != "mindweaver.core-surface.v1" || surface.Module != "github.com/mt-hub8/MindWeaver/v2" {
		return errors.New("identity is not frozen")
	}
	if len(surface.Routes) == 0 || len(surface.Routes) > maxContractRoutes {
		return errors.New("route count is outside the bounded CORE surface")
	}
	if !sortedUniqueStrings(surface.Packages) || !sortedUniqueStrings(surface.Migrations) ||
		!sortedUniqueStrings(surface.Tables) || !sortedUniqueStrings(surface.ForbiddenSegments) ||
		!sortedUniqueStrings(surface.ProductionDiscovery.EvidenceOnlyMainPackages) ||
		!sortedUniqueStrings(surface.ProductionDiscovery.ExternalModules) ||
		!sortedUniqueStrings(surface.ProductionDiscovery.LibraryRoots) ||
		!sortedUniqueStrings(surface.ProductionDiscovery.ExcludedRootPrefixes) ||
		!sortedUniqueStrings(surface.ProductionDiscovery.ExcludedFileSuffixes) {
		return errors.New("allowlists must be sorted and unique")
	}
	if !equalStrings(surface.ForbiddenSegments, requiredForbiddenSegments) {
		return errors.New("forbidden LATER segment policy drift")
	}
	for _, packagePath := range surface.ProductionDiscovery.EvidenceOnlyMainPackages {
		if !validSurfaceRelativePath(packagePath) || !hasSurfacePathPrefix(packagePath, surface.ProductionDiscovery.ExcludedRootPrefixes) {
			return fmt.Errorf("evidence-only main package %q is outside an excluded root", packagePath)
		}
		if segment, banned := forbiddenSegment(packagePath); banned {
			return fmt.Errorf("evidence-only main package contains forbidden segment %q", segment)
		}
	}
	for _, module := range surface.ProductionDiscovery.ExternalModules {
		modulePath, valid := parseExternalModuleIdentity(module)
		if !valid {
			return fmt.Errorf("invalid external module identity %q", module)
		}
		if segment, banned := forbiddenExternalModuleSegment(modulePath); banned {
			return fmt.Errorf("external module contains forbidden segment %q", segment)
		}
	}
	for _, packagePath := range surface.Packages {
		if segment, banned := forbiddenSegment(packagePath); banned {
			return fmt.Errorf("production package contains forbidden segment %q", segment)
		}
	}
	for _, migration := range surface.Migrations {
		if segment, banned := forbiddenSegment(migration); banned {
			return fmt.Errorf("migration contains forbidden segment %q", segment)
		}
	}
	for _, table := range surface.Tables {
		if segment, banned := forbiddenSegment(table); banned {
			return fmt.Errorf("table contains forbidden segment %q", segment)
		}
	}
	seen := make(map[string]struct{}, len(surface.Routes))
	previous := ""
	for _, route := range surface.Routes {
		if err := validPath(route.Path); err != nil {
			return err
		}
		if route.Method != http.MethodGet && route.Method != http.MethodHead && route.Method != http.MethodPost && route.Method != http.MethodPut && route.Method != http.MethodDelete {
			return fmt.Errorf("unsupported method %q", route.Method)
		}
		key := route.Path + "\x00" + route.Method
		if key <= previous {
			return errors.New("routes must be sorted by path then method and unique")
		}
		previous = key
		if _, duplicate := seen[route.OperationID]; duplicate || !validOperationID(route.OperationID) {
			return fmt.Errorf("duplicate or invalid operationId %q", route.OperationID)
		}
		seen[route.OperationID] = struct{}{}
		if route.Session != "none" && route.Session != "required" {
			return errors.New("invalid session policy")
		}
		if route.CSRF != "none" && route.CSRF != "required" {
			return errors.New("invalid CSRF policy")
		}
		if route.IdempotencyKey != "none" && route.IdempotencyKey != "required" {
			return errors.New("invalid idempotency policy")
		}
		if !sortedUniqueInts(route.SuccessStatuses) || !sortedUniqueInts(route.ProblemStatuses) || !sortedUniqueInts(route.TransportStatuses) {
			return errors.New("status allowlists must be sorted and unique")
		}
		if segment, banned := forbiddenSegment(route.Path); banned {
			return fmt.Errorf("route contains forbidden segment %q", segment)
		}
	}
	seenCommands := make(map[string]struct{})
	commandPackages := make(map[string]struct{})
	commandModules := make(map[string]struct{})
	previous = ""
	for _, command := range surface.Commands {
		if command.Name <= previous || command.Name == "" || strings.ContainsAny(command.Name, " /\\") {
			return errors.New("commands must be sorted, unique, and single path segments")
		}
		if !lowerHexSHA256(command.SourceManifestSHA256) {
			return fmt.Errorf("command %q must bind one lowercase SHA-256 source digest", command.Name)
		}
		previous = command.Name
		if _, exists := seenCommands[command.Name]; exists || !sortedUniqueStrings(command.Verbs) || !sortedUniqueStrings(command.Aliases) ||
			!sortedUniqueStrings(command.Packages) || !sortedUniqueStrings(command.ExternalModules) {
			return errors.New("invalid command allowlist")
		}
		seenCommands[command.Name] = struct{}{}
		for _, packagePath := range command.Packages {
			if !validSurfaceRelativePath(packagePath) || !hasSurfacePathPrefix(packagePath, surface.ProductionDiscovery.LibraryRoots) ||
				hasSurfacePathPrefix(packagePath, surface.ProductionDiscovery.ExcludedRootPrefixes) {
				return fmt.Errorf("command %q contains an invalid package path", command.Name)
			}
			if segment, banned := forbiddenSegment(packagePath); banned {
				return fmt.Errorf("command %q package contains forbidden segment %q", command.Name, segment)
			}
			commandPackages[packagePath] = struct{}{}
		}
		for _, module := range command.ExternalModules {
			modulePath, valid := parseExternalModuleIdentity(module)
			if !valid {
				return fmt.Errorf("command %q contains an invalid external module identity", command.Name)
			}
			if segment, banned := forbiddenExternalModuleSegment(modulePath); banned {
				return fmt.Errorf("command %q external module contains forbidden segment %q", command.Name, segment)
			}
			commandModules[module] = struct{}{}
		}
		for verb, nested := range command.NestedVerbs {
			if verb == "" || !sortedUniqueStrings(nested) {
				return errors.New("invalid nested command allowlist")
			}
		}
		if segment, banned := forbiddenSegment(command.Name); banned {
			return fmt.Errorf("command contains forbidden segment %q", segment)
		}
		for _, verb := range command.Verbs {
			if segment, banned := forbiddenSegment(verb); banned {
				return fmt.Errorf("command verb contains forbidden segment %q", segment)
			}
		}
		for _, nested := range command.NestedVerbs {
			for _, verb := range nested {
				if segment, banned := forbiddenSegment(verb); banned {
					return fmt.Errorf("nested command verb contains forbidden segment %q", segment)
				}
			}
		}
	}
	if !equalStrings(sortedStringKeys(commandPackages), surface.Packages) {
		return errors.New("command package ownership does not match the global production package set")
	}
	if !equalStrings(sortedStringKeys(commandModules), surface.ProductionDiscovery.ExternalModules) {
		return errors.New("command external-module ownership does not match the global production module set")
	}
	return nil
}

func sortedStringKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func lowerHexSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	nonZero := false
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
		nonZero = nonZero || character != '0'
	}
	return nonZero
}

func validSurfaceRelativePath(value string) bool {
	if value == "" || len(value) > 512 || strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") ||
		strings.Contains(value, "\\") || strings.Contains(value, "//") || strings.ContainsAny(value, "\x00\r\n\t:@#") {
		return false
	}
	for _, character := range []byte(value) {
		if character <= 0x20 || character >= 0x7f {
			return false
		}
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func hasSurfacePathPrefix(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if value == prefix || strings.HasPrefix(value, prefix+"/") {
			return true
		}
	}
	return false
}

func parseExternalModuleIdentity(value string) (string, bool) {
	hashSeparator := strings.LastIndex(value, "#h1:")
	versionSeparator := strings.LastIndex(value, "@")
	if versionSeparator <= 0 || hashSeparator <= versionSeparator+2 || hashSeparator+4 >= len(value) {
		return "", false
	}
	modulePath := value[:versionSeparator]
	version := value[versionSeparator+1 : hashSeparator]
	if !validSurfaceRelativePath(modulePath) || !validCanonicalModuleVersion(version) {
		return "", false
	}
	encoded := value[hashSeparator+4:]
	digest, err := base64.StdEncoding.Strict().DecodeString(encoded)
	return modulePath, err == nil && len(digest) == 32 && base64.StdEncoding.EncodeToString(digest) == encoded
}

func validCanonicalModuleVersion(version string) bool {
	if len(version) < len("v0.0.0") || len(version) > 128 || version[0] != 'v' {
		return false
	}
	withoutBuild, build, hasBuild := strings.Cut(version[1:], "+")
	if hasBuild && build != "incompatible" {
		return false
	}
	core, prerelease, hasPrerelease := strings.Cut(withoutBuild, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if !canonicalNumericIdentifier(part) {
			return false
		}
	}
	if hasPrerelease && !validSemverIdentifiers(prerelease, true) {
		return false
	}
	return true
}

func canonicalNumericIdentifier(value string) bool {
	if value == "" || len(value) > 1 && value[0] == '0' {
		return false
	}
	for _, character := range []byte(value) {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func validSemverIdentifiers(value string, rejectNumericLeadingZero bool) bool {
	if value == "" {
		return false
	}
	for _, identifier := range strings.Split(value, ".") {
		if identifier == "" || rejectNumericLeadingZero && len(identifier) > 1 && identifier[0] == '0' && allASCIIDigits(identifier) {
			return false
		}
		for _, character := range []byte(identifier) {
			if (character < '0' || character > '9') && (character < 'A' || character > 'Z') &&
				(character < 'a' || character > 'z') && character != '-' {
				return false
			}
		}
	}
	return true
}

func allASCIIDigits(value string) bool {
	for _, character := range []byte(value) {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func forbiddenSegment(value string) (string, bool) {
	return forbiddenSegmentFrom(value, requiredForbiddenSegments)
}

func forbiddenExternalModuleSegment(value string) (string, bool) {
	if segment, forbidden := forbiddenSegment(value); forbidden {
		return segment, true
	}
	return forbiddenSegmentFrom(value, forbiddenLegacyBackendModuleSegments)
}

func forbiddenSegmentFrom(value string, segments []string) (string, bool) {
	forbidden := make(map[string]struct{}, len(segments))
	for _, segment := range segments {
		forbidden[segment] = struct{}{}
	}
	for _, segment := range strings.FieldsFunc(strings.ToLower(value), func(character rune) bool {
		return (character < 'a' || character > 'z') && (character < '0' || character > '9')
	}) {
		if _, banned := forbidden[segment]; banned {
			return segment, true
		}
	}
	return "", false
}

func decodeCanonicalStrict(raw []byte, destination any) error {
	if len(raw) == 0 || len(raw) > maxContractBytes || !utf8.Valid(raw) || bytes.Contains(raw, []byte{'\r'}) || raw[len(raw)-1] != '\n' {
		return errors.New("document must be bounded UTF-8 with LF and one final newline")
	}
	if err := rejectDuplicateJSON(raw); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON data")
	}
	var generic any
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&generic); err != nil {
		return err
	}
	canonical, err := json.MarshalIndent(generic, "", "  ")
	if err != nil {
		return err
	}
	canonical = append(canonical, '\n')
	if !bytes.Equal(raw, canonical) {
		index := firstDifference(raw, canonical)
		return fmt.Errorf("document is not canonical indented JSON at byte %d (size %d, canonical %d)", index, len(raw), len(canonical))
	}
	return nil
}

func firstDifference(left, right []byte) int {
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	for index := 0; index < limit; index++ {
		if left[index] != right[index] {
			return index
		}
	}
	return limit
}

type scanBudget struct{ tokens int }

func rejectDuplicateJSON(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	budget := &scanBudget{}
	if err := scanValue(decoder, 0, budget); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON data")
	}
	return nil
}

func scanValue(decoder *json.Decoder, depth int, budget *scanBudget) error {
	token, err := nextToken(decoder, budget)
	if err != nil {
		return err
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		return nil
	}
	if depth >= maxContractDepth {
		return errors.New("JSON nesting exceeds bound")
	}
	switch delimiter {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := nextToken(decoder, budget)
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("JSON object key is not text")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate JSON key %q", key)
			}
			seen[key] = struct{}{}
			if err := scanValue(decoder, depth+1, budget); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := scanValue(decoder, depth+1, budget); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid JSON delimiter")
	}
	_, err = nextToken(decoder, budget)
	return err
}

func nextToken(decoder *json.Decoder, budget *scanBudget) (json.Token, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	budget.tokens++
	if budget.tokens > maxContractTokens {
		return nil, errors.New("JSON token count exceeds bound")
	}
	return token, nil
}

func validPath(value string) error {
	if len(value) < 2 || len(value) > 160 || value[0] != '/' || strings.HasSuffix(value, "/") || strings.ContainsAny(value, "{}*%?#") {
		return fmt.Errorf("invalid or unbounded exact path %q", value)
	}
	for _, character := range value {
		if character < 0x21 || character > 0x7e {
			return fmt.Errorf("path is not printable ASCII %q", value)
		}
	}
	return nil
}

func validOperationID(value string) bool {
	if value == "" || len(value) > 64 || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, character := range value[1:] {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func sameInts(left, right []int) bool {
	left = append([]int(nil), left...)
	right = append([]int(nil), right...)
	sort.Ints(left)
	sort.Ints(right)
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sortedUniqueInts(values []int) bool {
	if len(values) == 0 {
		return true
	}
	for index, value := range values {
		if value < 100 || value > 599 || (index > 0 && values[index-1] >= value) {
			return false
		}
	}
	return true
}

func sortedUniqueStrings(values []string) bool {
	for index, value := range values {
		if value == "" || (index > 0 && values[index-1] >= value) {
			return false
		}
	}
	return true
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sortedMapKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func cloneStatusMap(source map[string]int) map[string]int {
	result := make(map[string]int, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
