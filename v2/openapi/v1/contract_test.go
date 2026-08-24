package v1_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/mt-hub8/MindWeaver/v2/internal/localhttp"
	"github.com/mt-hub8/MindWeaver/v2/internal/transport"
	contract "github.com/mt-hub8/MindWeaver/v2/openapi/v1"
)

func TestEmbeddedContractMatchesProduction(t *testing.T) {
	t.Parallel()
	snapshot, err := contract.ValidateEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	root := moduleRoot(t)
	assertProblemMapping(t, snapshot.ProblemStatusByCode)
	assertRouteSurface(t, root, snapshot.Routes)
	assertProductionPackages(t, root, snapshot.Surface)
	assertMigrationsAndTables(t, root, snapshot.Surface)
	assertCommandSurface(t, root, snapshot.Surface)
	assertNoLaterSurface(t, snapshot.Surface)
}

func TestContractFailsClosed(t *testing.T) {
	t.Parallel()
	openAPI, surface := contract.Documents()
	tests := []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{
			name: "unknown root field",
			mutate: func(raw []byte) []byte {
				return mutateObject(t, raw, func(document map[string]any) { document["unknown"] = true })
			},
		},
		{
			name: "unknown schema vocabulary",
			mutate: func(raw []byte) []byte {
				return mutateObject(t, raw, func(document map[string]any) {
					problem := document["components"].(map[string]any)["schemas"].(map[string]any)["Problem"].(map[string]any)
					problem["nullable"] = true
				})
			},
		},
		{
			name: "duplicate operation id",
			mutate: func(raw []byte) []byte {
				return mutateObject(t, raw, func(document map[string]any) {
					paths := document["paths"].(map[string]any)
					paths["/api/v1/runtime"].(map[string]any)["get"].(map[string]any)["operationId"] = "getDiagnostics"
				})
			},
		},
		{
			name: "unimplemented route",
			mutate: func(raw []byte) []byte {
				return mutateObject(t, raw, func(document map[string]any) {
					operation := cloneJSONValue(t, document["paths"].(map[string]any)["/api/v1/runtime"].(map[string]any)["get"])
					operation.(map[string]any)["operationId"] = "unimplemented"
					document["paths"].(map[string]any)["/api/v1/unimplemented"] = map[string]any{"get": operation}
				})
			},
		},
		{
			name: "unsupported method",
			mutate: func(raw []byte) []byte {
				return mutateObject(t, raw, func(document map[string]any) {
					item := document["paths"].(map[string]any)["/api/v1/runtime"].(map[string]any)
					item["trace"] = cloneJSONValue(t, item["get"])
				})
			},
		},
		{
			name: "unbounded path",
			mutate: func(raw []byte) []byte {
				return mutateObject(t, raw, func(document map[string]any) {
					operation := cloneJSONValue(t, document["paths"].(map[string]any)["/api/v1/runtime"].(map[string]any)["get"])
					operation.(map[string]any)["operationId"] = "tooLong"
					document["paths"].(map[string]any)["/api/v1/"+strings.Repeat("x", 200)] = map[string]any{"get": operation}
				})
			},
		},
		{
			name: "unresolved reference",
			mutate: func(raw []byte) []byte {
				return mutateObject(t, raw, func(document map[string]any) {
					response := document["paths"].(map[string]any)["/api/v1/runtime"].(map[string]any)["get"].(map[string]any)["responses"].(map[string]any)["200"].(map[string]any)
					response["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)["$ref"] = "#/components/schemas/Missing"
				})
			},
		},
		{
			name: "invented etag",
			mutate: func(raw []byte) []byte {
				return mutateObject(t, raw, func(document map[string]any) {
					operation := document["paths"].(map[string]any)["/api/v1/runtime"].(map[string]any)["get"].(map[string]any)
					operation["parameters"] = []any{map[string]any{
						"in": "header", "name": "If-Match", "schema": map[string]any{"type": "string"},
					}}
				})
			},
		},
		{
			name: "missing idempotency header",
			mutate: func(raw []byte) []byte {
				return mutateObject(t, raw, func(document map[string]any) {
					operation := document["paths"].(map[string]any)["/api/v1/ask"].(map[string]any)["post"].(map[string]any)
					parameters := operation["parameters"].([]any)
					operation["parameters"] = parameters[:1]
				})
			},
		},
		{
			name: "wrong problem response",
			mutate: func(raw []byte) []byte {
				return mutateObject(t, raw, func(document map[string]any) {
					responses := document["paths"].(map[string]any)["/api/v1/answers"].(map[string]any)["get"].(map[string]any)["responses"].(map[string]any)
					responses["503"].(map[string]any)["$ref"] = "#/components/responses/Problem500"
				})
			},
		},
		{
			name: "problem required field drift",
			mutate: func(raw []byte) []byte {
				return mutateObject(t, raw, func(document map[string]any) {
					problem := document["components"].(map[string]any)["schemas"].(map[string]any)["Problem"].(map[string]any)
					problem["required"] = []any{"code", "status", "title", "type"}
				})
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := contract.Validate(test.mutate(openAPI), surface); err == nil {
				t.Fatal("mutated OpenAPI unexpectedly validated")
			}
		})
	}
	t.Run("duplicate json key", func(t *testing.T) {
		needle := []byte("\"openapi\": \"3.1.0\"")
		duplicate := []byte("\"openapi\": \"3.1.0\",\n  \"openapi\": \"3.1.0\"")
		mutated := bytes.Replace(openAPI, needle, duplicate, 1)
		if bytes.Equal(mutated, openAPI) {
			t.Fatal("duplicate-key mutation did not apply")
		}
		if _, err := contract.Validate(mutated, surface); err == nil {
			t.Fatal("duplicate JSON key unexpectedly validated")
		}
	})
	t.Run("unknown surface field", func(t *testing.T) {
		mutated := mutateObject(t, surface, func(document map[string]any) { document["unknown"] = true })
		if _, err := contract.Validate(openAPI, mutated); err == nil {
			t.Fatal("unknown surface field unexpectedly validated")
		}
	})
	t.Run("forbidden production package", func(t *testing.T) {
		mutated := mutateObject(t, surface, func(document map[string]any) {
			packages := append(document["packages"].([]any), "internal/agent")
			sort.Slice(packages, func(left, right int) bool { return packages[left].(string) < packages[right].(string) })
			document["packages"] = packages
		})
		if _, err := contract.Validate(openAPI, mutated); err == nil {
			t.Fatal("forbidden production package unexpectedly validated")
		}
	})
}

func assertProblemMapping(t *testing.T, actual map[string]int) {
	t.Helper()
	codes := []transport.ErrorCode{
		transport.CodeInvalidArgument,
		transport.CodeUnauthenticated,
		transport.CodeForbidden,
		transport.CodeNotFound,
		transport.CodeConflict,
		transport.CodeResourceLimit,
		transport.CodeServiceUnavailable,
		transport.CodeInternal,
	}
	want := make(map[string]int, len(codes))
	for _, code := range codes {
		problem := transport.NewProblem(code, "safe", "request")
		if err := problem.Validate(); err != nil {
			t.Fatalf("production Problem %q: %v", code, err)
		}
		want[string(code)] = problem.Status
	}
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("Problem status mapping = %#v, want %#v", actual, want)
	}
}

type routeIdentity struct {
	Method      string
	Path        string
	OperationID string
}

func assertRouteSurface(t *testing.T, root string, routes []contract.SurfaceRoute) {
	t.Helper()
	actual := extractAPIRoutes(t, filepath.Join(root, "internal", "app", "api.go"))
	actual = append(actual,
		routeIdentity{Method: http.MethodPost, Path: localhttp.BootstrapExchangePath, OperationID: "exchangeBootstrap"},
		routeIdentity{Method: http.MethodGet, Path: localhttp.HealthPath, OperationID: "getHealth"},
		routeIdentity{Method: http.MethodHead, Path: localhttp.HealthPath, OperationID: "headHealth"},
		routeIdentity{Method: http.MethodGet, Path: localhttp.CSRFRefreshPath, OperationID: "refreshCSRF"},
	)
	want := make([]routeIdentity, 0, len(routes))
	for _, route := range routes {
		want = append(want, routeIdentity{Method: route.Method, Path: route.Path, OperationID: route.OperationID})
	}
	sortRoutes(actual)
	sortRoutes(want)
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("production routes = %#v, contract = %#v", actual, want)
	}

	idempotent := map[string]bool{
		"uploadDocument":     true,
		"createCollection":   true,
		"createConversation": true,
		"ask":                true,
	}
	for _, route := range routes {
		if (route.IdempotencyKey == "required") != idempotent[route.OperationID] {
			t.Fatalf("%s idempotency metadata drift", route.OperationID)
		}
		business := strings.HasPrefix(route.Path, "/api/v1/")
		wantSession := business || route.Path == localhttp.CSRFRefreshPath
		if wantSession != (route.Session == "required") {
			t.Fatalf("%s session metadata drift", route.OperationID)
		}
		wantCSRF := business && route.Method != http.MethodGet && route.Method != http.MethodHead
		if wantCSRF != (route.CSRF == "required") {
			t.Fatalf("%s CSRF metadata drift", route.OperationID)
		}
	}
}

func extractAPIRoutes(t *testing.T, filename string) []routeIdentity {
	t.Helper()
	contents, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), filename, contents, 0)
	if err != nil {
		t.Fatal(err)
	}
	handlerOperations := map[string]string{
		"runtime": "getRuntime", "diagnostics": "getDiagnostics", "upload": "uploadDocument",
		"documents": "listDocuments", "retryDocumentIngestion": "retryDocumentIngestion",
		"trashDocument": "trashDocument", "restoreDocument": "restoreDocument", "purgeDocument": "purgeDocument",
		"purgeStatus": "getPurgeStatus", "purges": "listPurges", "job": "getJob", "cancelJob": "cancelJob",
		"search": "search", "createCollection": "createCollection", "collections": "listCollections",
		"addCollectionMember": "addCollectionMember", "collectionMembers": "listCollectionMembers",
		"removeCollectionMember": "removeCollectionMember", "ollamaConfiguration": "getOllamaConfiguration",
		"configureOllama": "configureOllama", "probeOllama": "probeOllama",
		"createConversation": "createConversation", "conversations": "listConversations",
		"deleteConversation": "deleteConversation", "conversationMessages": "listConversationMessages",
		"ask": "ask", "answer": "getAnswer",
	}
	var result []routeIdentity
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "Register" || function.Recv == nil {
			continue
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			assignment, ok := node.(*ast.AssignStmt)
			if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
				return true
			}
			identifier, ok := assignment.Lhs[0].(*ast.Ident)
			literal, literalOK := assignment.Rhs[0].(*ast.CompositeLit)
			if !ok || !literalOK || identifier.Name != "routes" {
				return true
			}
			for _, element := range literal.Elts {
				routeLiteral, ok := element.(*ast.CompositeLit)
				if !ok || len(routeLiteral.Elts) != 3 {
					t.Fatalf("route registration is not a fixed three-field literal")
				}
				method := methodExpression(t, routeLiteral.Elts[0])
				path := pathExpression(t, routeLiteral.Elts[1])
				handler, ok := routeLiteral.Elts[2].(*ast.SelectorExpr)
				if !ok {
					t.Fatalf("route %s %s does not use an API method selector", method, path)
				}
				operationID, exists := handlerOperations[handler.Sel.Name]
				if !exists {
					t.Fatalf("handler %s is absent from the reviewed CORE operation map", handler.Sel.Name)
				}
				result = append(result, routeIdentity{Method: method, Path: path, OperationID: operationID})
			}
			return false
		})
	}
	if len(result) != 27 {
		t.Fatalf("extracted %d API routes, want 27", len(result))
	}
	return result
}

func methodExpression(t *testing.T, expression ast.Expr) string {
	t.Helper()
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok {
		t.Fatalf("route method is not an http.Method selector")
	}
	methods := map[string]string{
		"MethodGet": http.MethodGet, "MethodPost": http.MethodPost, "MethodPut": http.MethodPut,
		"MethodDelete": http.MethodDelete, "MethodHead": http.MethodHead,
	}
	method, ok := methods[selector.Sel.Name]
	if !ok {
		t.Fatalf("unsupported production method selector %s", selector.Sel.Name)
	}
	return method
}

func pathExpression(t *testing.T, expression ast.Expr) string {
	t.Helper()
	binary, ok := expression.(*ast.BinaryExpr)
	if !ok || binary.Op != token.ADD {
		t.Fatalf("route path is not apiPrefix plus a fixed string")
	}
	prefix, ok := binary.X.(*ast.Ident)
	suffix, suffixOK := binary.Y.(*ast.BasicLit)
	if !ok || prefix.Name != "apiPrefix" || !suffixOK || suffix.Kind != token.STRING {
		t.Fatalf("route path is not apiPrefix plus a fixed string")
	}
	decoded, err := strconv.Unquote(suffix.Value)
	if err != nil {
		t.Fatal(err)
	}
	return "/api/v1" + decoded
}

func assertProductionPackages(t *testing.T, root string, surface contract.Surface) {
	t.Helper()
	if !reflect.DeepEqual(surface.ProductionDiscovery.LibraryRoots, []string{"internal", "platform"}) ||
		!reflect.DeepEqual(surface.ProductionDiscovery.ExcludedFileSuffixes, []string{"_test.go"}) ||
		!reflect.DeepEqual(surface.ProductionDiscovery.ExcludedRootPrefixes, []string{"docs", "migration", "openapi", "qualification", "release", "spikes", "testdata"}) {
		t.Fatal("production discovery policy drift")
	}
	set := map[string]struct{}{}
	for _, libraryRoot := range surface.ProductionDiscovery.LibraryRoots {
		base := filepath.Join(root, filepath.FromSlash(libraryRoot))
		err := filepath.WalkDir(base, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
				return nil
			}
			relative, err := filepath.Rel(root, filepath.Dir(path))
			if err != nil {
				return err
			}
			set[filepath.ToSlash(relative)] = struct{}{}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	actual := sortedKeys(set)
	if !reflect.DeepEqual(actual, surface.Packages) {
		t.Fatalf("production packages = %#v, contract = %#v", actual, surface.Packages)
	}
}

func assertMigrationsAndTables(t *testing.T, root string, surface contract.Surface) {
	t.Helper()
	directory := filepath.Join(root, "internal", "store", "sqlite", "migrations")
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	var migrations []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".sql" {
			migrations = append(migrations, entry.Name())
		}
	}
	sort.Strings(migrations)
	if !reflect.DeepEqual(migrations, surface.Migrations) {
		t.Fatalf("migration files = %#v, contract = %#v", migrations, surface.Migrations)
	}
	tables := map[string]struct{}{}
	operation := regexp.MustCompile(`(?i)\b(?:CREATE\s+(?:VIRTUAL\s+)?TABLE\s+([a-z_][a-z0-9_]*)|DROP\s+TABLE\s+([a-z_][a-z0-9_]*)|ALTER\s+TABLE\s+([a-z_][a-z0-9_]*)\s+RENAME\s+TO\s+([a-z_][a-z0-9_]*))`)
	for _, migration := range migrations {
		contents, err := os.ReadFile(filepath.Join(directory, migration))
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range operation.FindAllStringSubmatch(string(contents), -1) {
			switch {
			case match[1] != "":
				tables[strings.ToLower(match[1])] = struct{}{}
			case match[2] != "":
				delete(tables, strings.ToLower(match[2]))
			case match[3] != "":
				delete(tables, strings.ToLower(match[3]))
				tables[strings.ToLower(match[4])] = struct{}{}
			}
		}
	}
	actual := sortedKeys(tables)
	if !reflect.DeepEqual(actual, surface.Tables) {
		t.Fatalf("declared final SQLite tables = %#v, contract = %#v", actual, surface.Tables)
	}
}

func assertCommandSurface(t *testing.T, root string, surface contract.Surface) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, "cmd"))
	if err != nil {
		t.Fatal(err)
	}
	var actualNames []string
	for _, entry := range entries {
		if entry.IsDir() {
			actualNames = append(actualNames, entry.Name())
		}
	}
	sort.Strings(actualNames)
	wantNames := make([]string, 0, len(surface.Commands))
	for _, command := range surface.Commands {
		wantNames = append(wantNames, command.Name)
	}
	if !reflect.DeepEqual(actualNames, wantNames) {
		t.Fatalf("command packages = %#v, contract = %#v", actualNames, wantNames)
	}
	mainSource := filepath.Join(root, "cmd", "mindweaver", "main.go")
	verbs := switchStringCases(t, mainSource, "run")
	nested := switchStringCases(t, mainSource, "runConfig")
	for _, command := range surface.Commands {
		if command.Name != "mindweaver" {
			if command.Public || len(command.Verbs) != 0 || len(command.Aliases) != 0 || len(command.NestedVerbs) != 0 {
				t.Fatal("internal PDF helper must not expose user command verbs")
			}
			continue
		}
		if !command.Public || command.DefaultVerb != "serve" || !bytes.Contains(mustRead(t, mainSource), []byte("if len(args) == 0 {\n\t\treturn runServe")) {
			t.Fatal("mindweaver default command drift")
		}
		aliases := []string{}
		publicVerbs := []string{}
		for _, verb := range verbs {
			if strings.HasPrefix(verb, "-") {
				aliases = append(aliases, verb)
			} else {
				publicVerbs = append(publicVerbs, verb)
			}
		}
		sort.Strings(aliases)
		sort.Strings(publicVerbs)
		if !reflect.DeepEqual(aliases, command.Aliases) || !reflect.DeepEqual(publicVerbs, command.Verbs) || !reflect.DeepEqual(nested, command.NestedVerbs["config"]) {
			t.Fatalf("mindweaver verbs = %#v/%#v/%#v, contract = %#v/%#v/%#v", publicVerbs, aliases, nested, command.Verbs, command.Aliases, command.NestedVerbs["config"])
		}
	}
}

func switchStringCases(t *testing.T, filename, functionName string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	set := map[string]struct{}{}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != functionName {
			continue
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			clause, ok := node.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, expression := range clause.List {
				literal, ok := expression.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					continue
				}
				value, err := strconv.Unquote(literal.Value)
				if err != nil {
					t.Fatal(err)
				}
				set[value] = struct{}{}
			}
			return true
		})
	}
	return sortedKeys(set)
}

func assertNoLaterSurface(t *testing.T, surface contract.Surface) {
	t.Helper()
	forbidden := make(map[string]struct{}, len(surface.ForbiddenSegments))
	for _, segment := range surface.ForbiddenSegments {
		forbidden[segment] = struct{}{}
	}
	check := func(kind, value string) {
		for _, segment := range strings.FieldsFunc(strings.ToLower(value), func(character rune) bool {
			return (character < 'a' || character > 'z') && (character < '0' || character > '9')
		}) {
			if _, banned := forbidden[segment]; banned {
				t.Errorf("%s %q contains forbidden LATER segment %q", kind, value, segment)
			}
		}
	}
	for _, packagePath := range surface.Packages {
		check("package", packagePath)
	}
	for _, route := range surface.Routes {
		check("route", route.Path)
		check("operation", route.OperationID)
	}
	for _, table := range surface.Tables {
		check("table", table)
	}
	for _, migration := range surface.Migrations {
		check("migration", migration)
	}
	for _, command := range surface.Commands {
		check("command", command.Name)
		for _, verb := range command.Verbs {
			check("command verb", verb)
		}
		for _, verbs := range command.NestedVerbs {
			for _, verb := range verbs {
				check("nested command verb", verb)
			}
		}
	}
}

func mutateObject(t *testing.T, raw []byte, mutate func(map[string]any)) []byte {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	mutate(document)
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(encoded, '\n')
}

func cloneJSONValue(t *testing.T, value any) any {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var clone any
	if err := json.Unmarshal(encoded, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func sortRoutes(routes []routeIdentity) {
	sort.Slice(routes, func(left, right int) bool {
		if routes[left].Path != routes[right].Path {
			return routes[left].Path < routes[right].Path
		}
		return routes[left].Method < routes[right].Method
	})
}

func sortedKeys[T any](values map[string]T) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func mustRead(t *testing.T, filename string) []byte {
	t.Helper()
	contents, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}

func ExampleValidateEmbedded() {
	snapshot, err := contract.ValidateEmbedded()
	if err != nil {
		panic(err)
	}
	fmt.Println(len(snapshot.Routes))
	// Output: 31
}
