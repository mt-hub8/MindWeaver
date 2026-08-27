package v1_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/mt-hub8/MindWeaver/v2/internal/localhttp"
	sqliteStore "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/internal/transport"
	contract "github.com/mt-hub8/MindWeaver/v2/openapi/v1"
	_ "github.com/ncruces/go-sqlite3/driver"
)

func TestEmbeddedContractMatchesProduction(t *testing.T) {
	t.Parallel()
	snapshot, err := contract.ValidateEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	root := moduleRoot(t)
	assertProblemMapping(t, snapshot.ProblemStatusByCode, snapshot.ProblemRecoveryByCode)
	assertRouteSurface(t, root, snapshot.Routes)
	inventories := assertProductionPackages(t, root, snapshot.Surface)
	assertNoLegacyMigrationSurface(t, root, inventories)
	assertMigrationsAndTables(t, root, snapshot.Surface)
	assertCommandSurface(t, root, snapshot.Surface, inventories)
	assertNoLaterSurface(t, snapshot.Surface)
}

func TestBackupCreateContractDoesNotClaimExistingTargetAsFresh(t *testing.T) {
	t.Parallel()
	openAPI, _ := contract.Documents()
	var document map[string]any
	if err := json.Unmarshal(openAPI, &document); err != nil {
		t.Fatal(err)
	}
	operation := document["paths"].(map[string]any)["/api/v1/backups"].(map[string]any)["post"].(map[string]any)
	description := operation["description"].(string)
	summary := operation["summary"].(string)
	retry := operation["x-mindweaver-retry-safety"].(string)
	responses := operation["responses"].(map[string]any)
	if !strings.Contains(description, "never claimed as a fresh snapshot") ||
		!strings.Contains(description, "plaintext SQLite and blob files") ||
		!strings.Contains(summary, "no-replace, integrity-verifiable") || strings.Contains(summary, "immutable") ||
		!strings.Contains(retry, "bounded process-memory") ||
		!strings.Contains(retry, "history exhaustion fails closed") || responses["413"] == nil {
		t.Fatalf("backup retry contract is not fail-closed: description=%q retry=%q", description, retry)
	}
	failureCode := document["components"].(map[string]any)["schemas"].(map[string]any)["BackupOperationStatus"].(map[string]any)["properties"].(map[string]any)["failureCode"].(map[string]any)["enum"].([]any)
	if !slices.Contains(failureCode, any("BACKUP_EXISTING_VERIFIED")) {
		t.Fatal("backup status contract omits BACKUP_EXISTING_VERIFIED")
	}
	status := document["paths"].(map[string]any)["/api/v1/backups/status"].(map[string]any)["get"].(map[string]any)
	cancel := document["paths"].(map[string]any)["/api/v1/backups/cancel"].(map[string]any)["post"].(map[string]any)
	cancelDescription := cancel["responses"].(map[string]any)["202"].(map[string]any)["description"].(string)
	if !strings.Contains(status["summary"].(string), "retained process-local") ||
		!strings.Contains(status["responses"].(map[string]any)["200"].(map[string]any)["description"].(string), "retained process-local") ||
		strings.Contains(cancel["summary"].(string), "active backup") ||
		!strings.Contains(cancelDescription, "active operation") ||
		!strings.Contains(cancelDescription, "retained terminal operation") {
		t.Fatal("backup status/cancel contract overstates a single current operation")
	}
}

func TestSQLiteBackedReadOperationsDeclareRetryableContention(t *testing.T) {
	t.Parallel()
	openAPI, _ := contract.Documents()
	var document map[string]any
	if err := json.Unmarshal(openAPI, &document); err != nil {
		t.Fatal(err)
	}
	paths := document["paths"].(map[string]any)
	for operationID, path := range map[string]string{
		"listCollections": "/api/v1/collections",
		"listDocuments":   "/api/v1/documents",
		"getPurgeStatus":  "/api/v1/documents/purge-status",
		"listPurges":      "/api/v1/documents/purges",
	} {
		operation := paths[path].(map[string]any)["get"].(map[string]any)
		responses := operation["responses"].(map[string]any)
		serviceUnavailable, ok := responses["503"].(map[string]any)
		if operation["operationId"] != operationID || operation["x-mindweaver-retry-safety"] != "read-only" ||
			!ok || serviceUnavailable["$ref"] != "#/components/responses/Problem503" {
			t.Fatalf("%s contention contract = %#v", operationID, operation)
		}
	}
}

func TestOpenAPIDisclosesLiteralPhraseRetrievalBoundary(t *testing.T) {
	t.Parallel()
	openAPI, _ := contract.Documents()
	var document map[string]any
	if err := json.Unmarshal(openAPI, &document); err != nil {
		t.Fatal(err)
	}
	schemas := document["components"].(map[string]any)["schemas"].(map[string]any)
	askQuestion := schemas["AskRequest"].(map[string]any)["properties"].(map[string]any)["question"].(map[string]any)
	answerQuestion := schemas["Answer"].(map[string]any)["properties"].(map[string]any)["question"].(map[string]any)
	if askQuestion["minLength"] != float64(3) || answerQuestion["minLength"] != float64(1) {
		t.Fatalf("question minimums = AskRequest %v, Answer %v", askQuestion["minLength"], answerQuestion["minLength"])
	}
	for name, description := range map[string]string{
		"Ask request question": askQuestion["description"].(string),
		"Answer question":      answerQuestion["description"].(string),
	} {
		description = strings.ToLower(description)
		for _, required := range []string{"complete value unchanged", "literal continuous source phrase", "natural-question"} {
			if !strings.Contains(description, required) {
				t.Fatalf("%s description %q omits %q", name, description, required)
			}
		}
	}

	paths := document["paths"].(map[string]any)
	ask := paths["/api/v1/ask"].(map[string]any)["post"].(map[string]any)
	search := paths["/api/v1/search"].(map[string]any)["get"].(map[string]any)
	q := search["parameters"].([]any)[0].(map[string]any)
	for name, value := range map[string]string{
		"Ask operation":    ask["description"].(string),
		"Search operation": search["description"].(string),
		"Search q":         q["description"].(string),
	} {
		lower := strings.ToLower(value)
		if !strings.Contains(lower, "literal continuous") || !strings.Contains(lower, "complete") {
			t.Fatalf("%s does not disclose complete literal-continuous input: %q", name, value)
		}
	}
	if !strings.Contains(ask["description"].(string), "NO_CONTEXT") ||
		!strings.Contains(ask["description"].(string), "does not claim natural-language retrieval") ||
		!strings.Contains(search["description"].(string), "semantic retrieval are unsupported") ||
		!strings.Contains(q["description"].(string), "two-code-point queries are unsupported") {
		t.Fatal("OpenAPI overstates natural-question or two-code-point retrieval")
	}
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
		{
			name: "closed success response",
			mutate: func(raw []byte) []byte {
				return mutateObject(t, raw, func(document map[string]any) {
					schema := document["components"].(map[string]any)["schemas"].(map[string]any)["AnswerEnvelope"].(map[string]any)
					schema["additionalProperties"] = false
				})
			},
		},
		{
			name: "Problem recovery decision drift",
			mutate: func(raw []byte) []byte {
				return mutateObject(t, raw, func(document map[string]any) {
					problem := document["components"].(map[string]any)["schemas"].(map[string]any)["Problem"].(map[string]any)
					recovery := problem["x-mindweaver-recovery-by-code"].(map[string]any)
					recovery["CONFLICT"].(map[string]any)["userAction"] = "retry_later"
				})
			},
		},
		{
			name: "Problem false retryability missing",
			mutate: func(raw []byte) []byte {
				return mutateObject(t, raw, func(document map[string]any) {
					problem := document["components"].(map[string]any)["schemas"].(map[string]any)["Problem"].(map[string]any)
					recovery := problem["x-mindweaver-recovery-by-code"].(map[string]any)
					delete(recovery["CONFLICT"].(map[string]any), "retryable")
				})
			},
		},
		{
			name: "open strict request",
			mutate: func(raw []byte) []byte {
				return mutateObject(t, raw, func(document map[string]any) {
					schema := document["components"].(map[string]any)["schemas"].(map[string]any)["AskRequest"].(map[string]any)
					schema["additionalProperties"] = true
				})
			},
		},
		{
			name: "closed inline success response",
			mutate: func(raw []byte) []byte {
				return mutateObject(t, raw, func(document map[string]any) {
					page := document["components"].(map[string]any)["schemas"].(map[string]any)["CollectionMembersPage"].(map[string]any)
					members := page["properties"].(map[string]any)["members"].(map[string]any)
					members["items"].(map[string]any)["additionalProperties"] = false
				})
			},
		},
		{
			name: "pending Answer completedAt drift",
			mutate: func(raw []byte) []byte {
				return mutateObject(t, raw, func(document map[string]any) {
					answer := document["components"].(map[string]any)["schemas"].(map[string]any)["Answer"].(map[string]any)
					pending := answer["oneOf"].([]any)[0].(map[string]any)["properties"].(map[string]any)
					pending["completedAt"] = map[string]any{"format": "date-time", "type": "string"}
				})
			},
		},
		{
			name: "Answer parent completedAt drift",
			mutate: func(raw []byte) []byte {
				return mutateObject(t, raw, func(document map[string]any) {
					answer := document["components"].(map[string]any)["schemas"].(map[string]any)["Answer"].(map[string]any)
					answer["properties"].(map[string]any)["completedAt"] = map[string]any{"format": "date-time", "type": "string"}
				})
			},
		},
		{
			name: "Message citation reference drift",
			mutate: func(raw []byte) []byte {
				return mutateObject(t, raw, func(document map[string]any) {
					message := document["components"].(map[string]any)["schemas"].(map[string]any)["Message"].(map[string]any)
					citations := message["properties"].(map[string]any)["citations"].(map[string]any)
					citations["items"].(map[string]any)["$ref"] = "#/components/schemas/Identifier"
				})
			},
		},
		{
			name: "user Message recovery drift",
			mutate: func(raw []byte) []byte {
				return mutateObject(t, raw, func(document map[string]any) {
					message := document["components"].(map[string]any)["schemas"].(map[string]any)["Message"].(map[string]any)
					user := message["oneOf"].([]any)[0].(map[string]any)["properties"].(map[string]any)
					user["reconcileAfter"] = map[string]any{"format": "date-time", "type": "string"}
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
	for _, packagePath := range []string{
		"internal/agent", "internal/batchrunner", "internal/cachestore", "internal/hybridsearch",
		"internal/kbhealth", "internal/qdrantclient", "internal/queryunderstandingservice", "internal/storagesummary",
	} {
		t.Run("forbidden production package "+packagePath, func(t *testing.T) {
			mutated := mutateObject(t, surface, func(document map[string]any) {
				packages := append(document["packages"].([]any), packagePath)
				sort.Slice(packages, func(left, right int) bool { return packages[left].(string) < packages[right].(string) })
				document["packages"] = packages
				command := document["commands"].([]any)[0].(map[string]any)
				commandPackages := append(command["packages"].([]any), packagePath)
				sort.Slice(commandPackages, func(left, right int) bool {
					return commandPackages[left].(string) < commandPackages[right].(string)
				})
				command["packages"] = commandPackages
			})
			if _, err := contract.Validate(openAPI, mutated); err == nil {
				t.Fatalf("forbidden production package %q unexpectedly validated", packagePath)
			} else if !strings.Contains(err.Error(), "forbidden segment") {
				t.Fatalf("forbidden production package %q failed for the wrong reason: %v", packagePath, err)
			}
		})
	}
	t.Run("all zero command source digest", func(t *testing.T) {
		mutated := mutateObject(t, surface, func(document map[string]any) {
			document["commands"].([]any)[0].(map[string]any)["sourceManifestSHA256"] = strings.Repeat("0", 64)
		})
		if _, err := contract.Validate(openAPI, mutated); err == nil {
			t.Fatal("all-zero command source digest unexpectedly validated")
		}
	})
	t.Run("invalid external module identity", func(t *testing.T) {
		mutated := mutateObject(t, surface, func(document map[string]any) {
			discovery := document["productionDiscovery"].(map[string]any)
			discovery["externalModules"] = []any{"github.com/example/mysql@v1.0.0#h1:not-a-sha256"}
		})
		if _, err := contract.Validate(openAPI, mutated); err == nil {
			t.Fatal("invalid external module identity unexpectedly validated")
		}
	})
	t.Run("non-semver external module version", func(t *testing.T) {
		mutated := mutateObject(t, surface, func(document map[string]any) {
			discovery := document["productionDiscovery"].(map[string]any)
			modules := discovery["externalModules"].([]any)
			original := modules[0].(string)
			at := strings.LastIndex(original, "@")
			hash := strings.LastIndex(original, "#h1:")
			invalid := original[:at] + "@vgarbage" + original[hash:]
			modules[0] = invalid
			sort.Slice(modules, func(left, right int) bool { return modules[left].(string) < modules[right].(string) })
			for _, rawCommand := range document["commands"].([]any) {
				command := rawCommand.(map[string]any)
				owned := command["externalModules"].([]any)
				for index := range owned {
					if owned[index] == original {
						owned[index] = invalid
					}
				}
				sort.Slice(owned, func(left, right int) bool { return owned[left].(string) < owned[right].(string) })
			}
		})
		if _, err := contract.Validate(openAPI, mutated); err == nil {
			t.Fatal("non-semver external module version unexpectedly validated")
		}
	})
	t.Run("forbidden external module terminal segment", func(t *testing.T) {
		mutated := mutateObject(t, surface, func(document map[string]any) {
			discovery := document["productionDiscovery"].(map[string]any)
			discovery["externalModules"] = []any{"github.com/acme/agent@v1.0.0#h1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}
		})
		if _, err := contract.Validate(openAPI, mutated); err == nil {
			t.Fatal("forbidden external module terminal segment unexpectedly validated")
		}
	})
	t.Run("forbidden legacy backend external module", func(t *testing.T) {
		const mysqlModule = "github.com/go-sql-driver/mysql@v1.9.3#h1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
		mutated := mutateObject(t, surface, func(document map[string]any) {
			discovery := document["productionDiscovery"].(map[string]any)
			modules := append(discovery["externalModules"].([]any), mysqlModule)
			sort.Slice(modules, func(left, right int) bool { return modules[left].(string) < modules[right].(string) })
			discovery["externalModules"] = modules
			command := document["commands"].([]any)[0].(map[string]any)
			commandModules := append(command["externalModules"].([]any), mysqlModule)
			sort.Slice(commandModules, func(left, right int) bool { return commandModules[left].(string) < commandModules[right].(string) })
			command["externalModules"] = commandModules
		})
		if _, err := contract.Validate(openAPI, mutated); err == nil {
			t.Fatal("forbidden legacy backend external module unexpectedly validated")
		}
	})
	t.Run("evidence main outside excluded roots", func(t *testing.T) {
		mutated := mutateObject(t, surface, func(document map[string]any) {
			discovery := document["productionDiscovery"].(map[string]any)
			discovery["evidenceOnlyMainPackages"] = []any{"tools/hidden"}
		})
		if _, err := contract.Validate(openAPI, mutated); err == nil {
			t.Fatal("evidence-only main outside excluded roots unexpectedly validated")
		}
	})
	t.Run("missing command package ownership", func(t *testing.T) {
		mutated := mutateObject(t, surface, func(document map[string]any) {
			command := document["commands"].([]any)[0].(map[string]any)
			packages := command["packages"].([]any)
			command["packages"] = append([]any(nil), packages[1:]...)
		})
		if _, err := contract.Validate(openAPI, mutated); err == nil {
			t.Fatal("missing command package ownership unexpectedly validated")
		}
	})
	t.Run("missing command module ownership", func(t *testing.T) {
		mutated := mutateObject(t, surface, func(document map[string]any) {
			command := document["commands"].([]any)[0].(map[string]any)
			modules := command["externalModules"].([]any)
			command["externalModules"] = append([]any(nil), modules[1:]...)
		})
		if _, err := contract.Validate(openAPI, mutated); err == nil {
			t.Fatal("missing command module ownership unexpectedly validated")
		}
	})
	t.Run("global package without command owner", func(t *testing.T) {
		mutated := mutateObject(t, surface, func(document map[string]any) {
			packages := append(document["packages"].([]any), "internal/unused")
			sort.Slice(packages, func(left, right int) bool { return packages[left].(string) < packages[right].(string) })
			document["packages"] = packages
		})
		if _, err := contract.Validate(openAPI, mutated); err == nil {
			t.Fatal("global package without command owner unexpectedly validated")
		}
	})
	t.Run("command package outside library roots", func(t *testing.T) {
		mutated := mutateObject(t, surface, func(document map[string]any) {
			command := document["commands"].([]any)[0].(map[string]any)
			commandPackages := command["packages"].([]any)
			commandPackages[0] = "cmd/mindweaver"
			sort.Slice(commandPackages, func(left, right int) bool { return commandPackages[left].(string) < commandPackages[right].(string) })
			packages := document["packages"].([]any)
			for index := range packages {
				if packages[index] == "internal/app" {
					packages[index] = "cmd/mindweaver"
				}
			}
			sort.Slice(packages, func(left, right int) bool { return packages[left].(string) < packages[right].(string) })
		})
		if _, err := contract.Validate(openAPI, mutated); err == nil {
			t.Fatal("command package outside library roots unexpectedly validated")
		}
	})
}

func assertProblemMapping(t *testing.T, actual map[string]int, actualRecovery map[string]contract.ProblemRecovery) {
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
	wantRecovery := make(map[string]contract.ProblemRecovery, len(codes))
	for _, code := range codes {
		problem := transport.NewProblem(code, "safe", "request")
		if err := problem.Validate(); err != nil {
			t.Fatalf("production Problem %q: %v", code, err)
		}
		want[string(code)] = problem.Status
		wantRecovery[string(code)] = contract.ProblemRecovery{
			Retryable: problem.Retryable, UserAction: string(problem.UserAction),
		}
	}
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("Problem status mapping = %#v, want %#v", actual, want)
	}
	if !reflect.DeepEqual(actualRecovery, wantRecovery) {
		t.Fatalf("Problem recovery mapping = %#v, want %#v", actualRecovery, wantRecovery)
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
		"createBackup":       true,
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
		"createBackup": "createBackup", "backupStatus": "getBackupStatus", "cancelBackup": "cancelBackup",
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
	if len(result) != 30 {
		t.Fatalf("extracted %d API routes, want 30", len(result))
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

func assertProductionPackages(t *testing.T, root string, surface contract.Surface) map[string]commandProductionInventory {
	t.Helper()
	if !reflect.DeepEqual(surface.ProductionDiscovery.EvidenceOnlyMainPackages, []string{"qualification/pdf/adversarialprobe", "qualification/runtime/pdfblocker", "spikes/sqlite/cmd/sqlite-spike", "tests/browser/runner"}) ||
		!reflect.DeepEqual(surface.ProductionDiscovery.LibraryRoots, []string{"internal", "platform"}) ||
		!reflect.DeepEqual(surface.ProductionDiscovery.ExcludedFileSuffixes, []string{"_test.go"}) ||
		!reflect.DeepEqual(surface.ProductionDiscovery.ExcludedRootPrefixes, []string{"docs", "openapi", "qualification", "release", "spikes", "testdata", "tests"}) {
		t.Fatal("production discovery policy drift")
	}
	commandNames := make([]string, 0, len(surface.Commands))
	commandContracts := make(map[string]contract.SurfaceCommand, len(surface.Commands))
	for _, command := range surface.Commands {
		commandNames = append(commandNames, command.Name)
		commandContracts[command.Name] = command
	}
	inventories := make(map[string]commandProductionInventory, len(commandNames))
	packageUnion := map[string]struct{}{}
	moduleUnion := map[string]struct{}{}
	for _, commandName := range commandNames {
		inventory, err := inspectCommandProduction(root, surface.Module, commandName, surface.ProductionDiscovery.LibraryRoots, surface.ProductionDiscovery.ExcludedRootPrefixes)
		if err != nil {
			t.Fatal(err)
		}
		command := commandContracts[commandName]
		if !reflect.DeepEqual(inventory.packages, command.Packages) {
			t.Fatalf("command %s packages = %#v, contract = %#v", commandName, inventory.packages, command.Packages)
		}
		if !reflect.DeepEqual(inventory.externalModules, command.ExternalModules) {
			t.Fatalf("command %s external modules = %#v, contract = %#v", commandName, inventory.externalModules, command.ExternalModules)
		}
		if inventory.sourceDigest != command.SourceManifestSHA256 {
			t.Fatalf("command %s transitive source manifest SHA-256 = %s, contract = %s", commandName, inventory.sourceDigest, command.SourceManifestSHA256)
		}
		inventories[commandName] = inventory
		for _, packagePath := range inventory.packages {
			packageUnion[packagePath] = struct{}{}
		}
		for _, module := range inventory.externalModules {
			moduleUnion[module] = struct{}{}
		}
	}
	actual := sortedKeys(packageUnion)
	if !reflect.DeepEqual(actual, surface.Packages) {
		t.Fatalf("production packages = %#v, contract = %#v", actual, surface.Packages)
	}
	externalModules := sortedKeys(moduleUnion)
	if !reflect.DeepEqual(externalModules, surface.ProductionDiscovery.ExternalModules) {
		t.Fatalf("external production modules = %#v, contract = %#v", externalModules, surface.ProductionDiscovery.ExternalModules)
	}
	actualMains, err := moduleMainPackages(root, surface.Module)
	if err != nil {
		t.Fatal(err)
	}
	expectedMains := append([]string(nil), surface.ProductionDiscovery.EvidenceOnlyMainPackages...)
	for _, command := range commandNames {
		expectedMains = append(expectedMains, "cmd/"+command)
	}
	sort.Strings(expectedMains)
	if !reflect.DeepEqual(actualMains, expectedMains) {
		t.Fatalf("module main packages = %#v, exact command/evidence allowlist = %#v", actualMains, expectedMains)
	}
	allSourceMains, err := sourceMainPackages(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(allSourceMains, expectedMains) {
		t.Fatalf("all source main packages = %#v, exact command/evidence allowlist = %#v", allSourceMains, expectedMains)
	}
	return inventories
}

func assertNoLegacyMigrationSurface(t *testing.T, root string, inventories map[string]commandProductionInventory) {
	t.Helper()
	if err := legacyMigrationSurfaceError(root); err != nil {
		t.Fatal(err)
	}
	if err := legacyMigrationSelectedFileError(inventories); err != nil {
		t.Fatal(err)
	}
}

var legacyMigrationTokens = [][]byte{
	[]byte("mindweaver-migrate"),
	[]byte("mindweaver-neutral-export"),
	[]byte("mindweaver-neutral-export/v1"),
	[]byte("legacyimport"),
	[]byte("legacy_import"),
	[]byte("legacy_ollama_intents"),
	[]byte("legacy_ollama_reconfigure_required"),
	[]byte("007_legacy_import"),
}

func legacyMigrationSurfaceError(root string) error {
	for _, relative := range []string{
		"migration",
		"internal/migration",
		"cmd/mindweaver-migrate",
		"internal/store/sqlite/legacy_import.go",
		"internal/store/sqlite/legacy_import_test.go",
		"internal/store/sqlite/migrations/007_legacy_import.sql",
	} {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("legacy migration surface exists: %s", relative)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect forbidden legacy migration surface %s: %w", relative, err)
		}
	}

	for _, relative := range []string{"cmd", "internal", "platform"} {
		base := filepath.Join(root, relative)
		err := filepath.WalkDir(base, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("production source tree contains link: %s", filepath.ToSlash(path))
			}
			if entry.IsDir() {
				return nil
			}
			lowerName := strings.ToLower(entry.Name())
			if strings.HasPrefix(lowerName, "legacy_import") {
				return fmt.Errorf("legacy import adapter exists: %s", filepath.ToSlash(path))
			}
			if filepath.Ext(lowerName) != ".go" && filepath.Ext(lowerName) != ".sql" {
				return nil
			}
			if strings.HasSuffix(lowerName, "_test.go") {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.Size() > 4<<20 {
				return fmt.Errorf("production source exceeds absence-scan bound: %s", filepath.ToSlash(path))
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if forbidden, found := legacyMigrationToken(contents); found {
				return fmt.Errorf("legacy migration token %q exists in %s", forbidden, filepath.ToSlash(path))
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func legacyMigrationSelectedFileError(inventories map[string]commandProductionInventory) error {
	seen := map[string]string{}
	for _, inventory := range inventories {
		for _, file := range inventory.selectedFiles {
			if previous, exists := seen[file.relative]; exists {
				if previous != file.digest {
					return errors.New("selected production file identity differs between commands")
				}
				continue
			}
			seen[file.relative] = file.digest
			if forbidden, found := legacyMigrationToken(file.contents); found {
				return fmt.Errorf("selected production file contains legacy migration token %q", forbidden)
			}
		}
	}
	return nil
}

func legacyMigrationToken(contents []byte) ([]byte, bool) {
	folded := append([]byte(nil), contents...)
	for index, value := range folded {
		if value >= 'A' && value <= 'Z' {
			folded[index] = value + ('a' - 'A')
		}
	}
	for _, forbidden := range legacyMigrationTokens {
		if bytes.Contains(folded, forbidden) {
			return forbidden, true
		}
	}
	return nil, false
}

func TestLegacyMigrationAbsenceGateRejectsForbiddenSurfaces(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		relative string
		contents string
	}{
		{name: "root package", relative: "migration/neutral.go", contents: "package migration\n"},
		{name: "internal package", relative: "internal/migration/import.go", contents: "package migration\n"},
		{name: "command", relative: "cmd/mindweaver-migrate/main.go", contents: "package main\n"},
		{name: "adapter case variant", relative: "internal/store/sqlite/Legacy_Import_Adapter.go", contents: "package sqlite\n"},
		{name: "schema", relative: "internal/store/sqlite/migrations/007_legacy_import.sql", contents: "SELECT 1;\n"},
		{name: "migrate command token", relative: "internal/store/sqlite/token.go", contents: "package sqlite\nconst stale = \"mindweaver-migrate\"\n"},
		{name: "neutral exporter token", relative: "internal/store/sqlite/token.go", contents: "package sqlite\nconst stale = \"mindweaver-neutral-export/v1\"\n"},
		{name: "legacy importer token", relative: "internal/store/sqlite/token.go", contents: "package sqlite\nconst stale = \"legacyimport\"\n"},
		{name: "legacy table token", relative: "internal/store/sqlite/token.go", contents: "package sqlite\nconst stale = \"legacy_imports\"\n"},
		{name: "legacy table token mixed case", relative: "internal/store/sqlite/token.go", contents: "package sqlite\nconst stale = \"Legacy_Import\"\n"},
		{name: "legacy Ollama table token", relative: "internal/store/sqlite/token.go", contents: "package sqlite\nconst stale = \"legacy_ollama_intents\"\n"},
		{name: "legacy reconfigure token", relative: "internal/store/sqlite/token.go", contents: "package sqlite\nconst stale = \"LEGACY_OLLAMA_RECONFIGURE_REQUIRED\"\n"},
		{name: "legacy schema token", relative: "internal/store/sqlite/token.go", contents: "package sqlite\nconst stale = \"007_legacy_import\"\n"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			for _, directory := range []string{"cmd", "internal", "platform"} {
				if err := os.MkdirAll(filepath.Join(root, directory), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(root, filepath.FromSlash(test.relative))
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(test.contents), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := legacyMigrationSurfaceError(root); err == nil {
				t.Fatal("forbidden legacy migration surface unexpectedly passed")
			}
		})
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
	databasePath := filepath.Join(t.TempDir(), sqliteStore.DatabaseFileName)
	store, err := sqliteStore.Open(context.Background(), databasePath, sqliteStore.Options{Connections: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.IntegrityCheck(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite3", readOnlySQLiteURI(t, databasePath))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	actual, err := applicationSchemaTables(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, surface.Tables) {
		t.Fatalf("fresh SQLite application tables = %#v, contract = %#v", actual, surface.Tables)
	}
}

func readOnlySQLiteURI(t *testing.T, filename string) string {
	t.Helper()
	absolute, err := filepath.Abs(filename)
	if err != nil {
		t.Fatal(err)
	}
	segments := strings.Split(filepath.ToSlash(absolute), "/")
	for index := range segments {
		segments[index] = url.PathEscape(segments[index])
	}
	return (&url.URL{Scheme: "file", Opaque: strings.Join(segments, "/"), RawQuery: "mode=ro"}).String()
}

func applicationSchemaTables(ctx context.Context, db *sql.DB) ([]string, error) {
	entries, err := sqliteSchemaTableEntries(ctx, db)
	if err != nil {
		return nil, err
	}
	return classifyApplicationSchema(entries)
}

type sqliteSchemaTableEntry struct {
	name string
	kind string
}

func sqliteSchemaTableEntries(ctx context.Context, db *sql.DB) ([]sqliteSchemaTableEntry, error) {
	rows, err := db.QueryContext(ctx, "SELECT name, type FROM pragma_table_list WHERE schema = 'main' ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]sqliteSchemaTableEntry, 0, 32)
	for rows.Next() {
		if len(entries) >= 256 {
			return nil, errors.New("SQLite schema object count exceeds its bound")
		}
		var entry sqliteSchemaTableEntry
		if err := rows.Scan(&entry.name, &entry.kind); err != nil {
			return nil, err
		}
		if len(entry.name) == 0 || len(entry.name) > 128 || len(entry.kind) == 0 || len(entry.kind) > 16 {
			return nil, errors.New("SQLite schema object identity is outside its bound")
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func classifyApplicationSchema(entries []sqliteSchemaTableEntry) ([]string, error) {
	requiredInternal := map[string]string{
		"sqlite_schema":   "table",
		"sqlite_sequence": "table",
	}
	requiredFTSStorage := map[string]string{
		"chunks_fts_config":  "table",
		"chunks_fts_data":    "table",
		"chunks_fts_docsize": "table",
		"chunks_fts_idx":     "table",
	}
	seen := make(map[string]struct{}, len(entries))
	caseFolded := make(map[string]string, len(entries))
	var tables []string
	for _, entry := range entries {
		if _, duplicate := seen[entry.name]; duplicate {
			return nil, errors.New("SQLite schema contains a duplicate object")
		}
		folded := strings.ToLower(entry.name)
		if previous, collision := caseFolded[folded]; collision && previous != entry.name {
			return nil, errors.New("SQLite schema contains an object-name case collision")
		}
		seen[entry.name] = struct{}{}
		caseFolded[folded] = entry.name
		if requiredKind, internal := requiredInternal[entry.name]; internal {
			if entry.kind != requiredKind {
				return nil, errors.New("SQLite internal object type drift")
			}
			continue
		}
		if strings.HasPrefix(folded, "sqlite_") {
			return nil, errors.New("SQLite internal object set drift")
		}
		if requiredKind, storage := requiredFTSStorage[entry.name]; storage {
			if entry.kind != requiredKind {
				return nil, fmt.Errorf("SQLite FTS storage object %q has type %q", entry.name, entry.kind)
			}
			continue
		}
		if entry.name == "chunks_fts" {
			if entry.kind != "virtual" {
				return nil, errors.New("SQLite FTS table is not virtual")
			}
		} else if entry.kind != "table" {
			return nil, errors.New("SQLite application object type drift")
		}
		tables = append(tables, entry.name)
	}
	for name := range requiredInternal {
		if _, exists := seen[name]; !exists {
			return nil, errors.New("SQLite internal object set is incomplete")
		}
	}
	for name := range requiredFTSStorage {
		if _, exists := seen[name]; !exists {
			return nil, errors.New("SQLite FTS storage object set is incomplete")
		}
	}
	if _, exists := seen["chunks_fts"]; !exists {
		return nil, errors.New("SQLite FTS virtual table is absent")
	}
	sort.Strings(tables)
	return tables, nil
}

func TestApplicationSchemaTablesIncludesQuotedIdentifiers(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE "agent_runs" (id INTEGER PRIMARY KEY) STRICT`); err != nil {
		t.Fatal(err)
	}
	entries, err := sqliteSchemaTableEntries(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries {
		found = found || entry.name == "agent_runs" && entry.kind == "table"
	}
	if !found {
		t.Fatalf("quoted SQLite table absent from inventory: %#v", entries)
	}
}

func TestApplicationSchemaClassificationFailsClosed(t *testing.T) {
	valid := []sqliteSchemaTableEntry{
		{name: "agent_runs", kind: "table"},
		{name: "chunks_fts", kind: "virtual"},
		{name: "chunks_fts_config", kind: "table"},
		{name: "chunks_fts_data", kind: "table"},
		{name: "chunks_fts_docsize", kind: "table"},
		{name: "chunks_fts_idx", kind: "table"},
		{name: "sqlite_schema", kind: "table"},
		{name: "sqlite_sequence", kind: "table"},
	}
	tables, err := classifyApplicationSchema(valid)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tables, []string{"agent_runs", "chunks_fts"}) {
		t.Fatalf("classified application tables = %#v", tables)
	}
	withUnexpectedContent := append(append([]sqliteSchemaTableEntry(nil), valid...), sqliteSchemaTableEntry{name: "chunks_fts_content", kind: "table"})
	tables, err = classifyApplicationSchema(withUnexpectedContent)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(tables, "chunks_fts_content") {
		t.Fatalf("unexpected FTS content table was hidden: %#v", tables)
	}
	for name, mutate := range map[string]func([]sqliteSchemaTableEntry) []sqliteSchemaTableEntry{
		"missing FTS storage": func(entries []sqliteSchemaTableEntry) []sqliteSchemaTableEntry {
			return append(entries[:2], entries[3:]...)
		},
		"reported shadow type drift": func(entries []sqliteSchemaTableEntry) []sqliteSchemaTableEntry {
			entries[2].kind = "shadow"
			return entries
		},
		"unexpected internal table": func(entries []sqliteSchemaTableEntry) []sqliteSchemaTableEntry {
			return append(entries, sqliteSchemaTableEntry{name: "sqlite_hidden", kind: "table"})
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := append([]sqliteSchemaTableEntry(nil), valid...)
			if _, err := classifyApplicationSchema(mutate(candidate)); err == nil {
				t.Fatal("invalid SQLite schema classification unexpectedly passed")
			}
		})
	}
}

func assertCommandSurface(t *testing.T, root string, surface contract.Surface, inventories map[string]commandProductionInventory) {
	t.Helper()
	if len(surface.Commands) != 2 || surface.Commands[0].Name != "mindweaver" || surface.Commands[1].Name != "mindweaver-pdf" {
		t.Fatalf("command contract must contain exactly mindweaver and mindweaver-pdf, got %#v", surface.Commands)
	}
	entries, err := os.ReadDir(filepath.Join(root, "cmd"))
	if err != nil {
		t.Fatal(err)
	}
	var actualNames []string
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			t.Fatalf("cmd root contains a non-directory or linked entry: %s", entry.Name())
		}
		actualNames = append(actualNames, entry.Name())
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
	nested := map[string][]string{
		"config":   switchStringCases(t, mainSource, "runConfig"),
		"recovery": switchStringCases(t, filepath.Join(root, "cmd", "mindweaver", "recovery.go"), "runRecovery"),
	}
	for _, command := range surface.Commands {
		if _, exists := inventories[command.Name]; !exists {
			t.Fatalf("command %s lacks a production inventory", command.Name)
		}
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
		if !reflect.DeepEqual(aliases, command.Aliases) || !reflect.DeepEqual(publicVerbs, command.Verbs) || !reflect.DeepEqual(nested, command.NestedVerbs) {
			t.Fatalf("mindweaver verbs = %#v/%#v/%#v, contract = %#v/%#v/%#v", publicVerbs, aliases, nested, command.Verbs, command.Aliases, command.NestedVerbs)
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
	current, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for depth := 0; depth < 8; depth++ {
		module, readErr := os.ReadFile(filepath.Join(current, "go.mod"))
		if readErr == nil && bytes.Contains(module, []byte("module github.com/mt-hub8/MindWeaver/v2")) {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	t.Fatal("locate v2 module root")
	return ""
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
	// Output: 34
}
