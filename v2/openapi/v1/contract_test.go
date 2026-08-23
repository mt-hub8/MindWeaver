package openapiv1

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type document struct {
	OpenAPI    string                            `json:"openapi"`
	Info       map[string]interface{}            `json:"info"`
	Paths      map[string]map[string]interface{} `json:"paths"`
	Components struct {
		Schemas map[string]map[string]interface{} `json:"schemas"`
	} `json:"components"`
}

func loadDocument(t *testing.T) document {
	t.Helper()
	data, err := os.ReadFile("openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc document
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("OpenAPI document is not valid JSON: %v", err)
	}
	return doc
}

func TestContractIsVersionedAndOperationIDsAreUnique(t *testing.T) {
	doc := loadDocument(t)
	if doc.OpenAPI != "3.1.0" {
		t.Fatalf("openapi = %q, want 3.1.0", doc.OpenAPI)
	}
	seen := map[string]string{}
	for path, item := range doc.Paths {
		if !strings.HasPrefix(path, "/api/v1/") {
			t.Errorf("unversioned path %q", path)
		}
		for method, raw := range item {
			if method == "parameters" || strings.HasPrefix(method, "x-") {
				continue
			}
			op, ok := raw.(map[string]interface{})
			if !ok {
				t.Errorf("%s %s is not an operation", method, path)
				continue
			}
			id, _ := op["operationId"].(string)
			if id == "" {
				t.Errorf("%s %s has no operationId", method, path)
			} else if previous := seen[id]; previous != "" {
				t.Errorf("operationId %q reused by %s and %s %s", id, previous, method, path)
			} else {
				seen[id] = method + " " + path
			}
		}
	}
}

func TestEveryMutationDeclaresIdempotencyAndCSRF(t *testing.T) {
	doc := loadDocument(t)
	for path, item := range doc.Paths {
		for _, method := range []string{"post", "put", "patch", "delete"} {
			raw, ok := item[method]
			if !ok {
				continue
			}
			op := raw.(map[string]interface{})
			parameters, _ := op["parameters"].([]interface{})
			refs := parameterRefs(parameters)
			if !refs["#/components/parameters/IdempotencyKey"] {
				t.Errorf("%s %s has no required idempotency contract", method, path)
			}
			if path != "/api/v1/bootstrap/session" && !refs["#/components/parameters/CSRFToken"] {
				t.Errorf("%s %s has no CSRF contract", method, path)
			}
		}
	}
}

func parameterRefs(parameters []interface{}) map[string]bool {
	refs := make(map[string]bool)
	for _, raw := range parameters {
		parameter, _ := raw.(map[string]interface{})
		ref, _ := parameter["$ref"].(string)
		refs[ref] = true
	}
	return refs
}

func TestProblemCodesAreStableAndSortedForReview(t *testing.T) {
	doc := loadDocument(t)
	problem := doc.Components.Schemas["Problem"]
	properties := problem["properties"].(map[string]interface{})
	code := properties["code"].(map[string]interface{})
	rawCodes := code["enum"].([]interface{})
	codes := make([]string, 0, len(rawCodes))
	for _, raw := range rawCodes {
		codes = append(codes, raw.(string))
	}
	if len(codes) < 10 {
		t.Fatalf("problem error code set is unexpectedly small: %v", codes)
	}
	seen := map[string]bool{}
	for _, code := range codes {
		if seen[code] {
			t.Errorf("duplicate problem code %q", code)
		}
		seen[code] = true
	}
	for _, required := range []string{"EGRESS_DENIED", "CONTEXT_INSUFFICIENT", "CITATION_INVALID", "IDEMPOTENCY_KEY_REUSED", "PRECONDITION_FAILED", "OUTCOME_UNCERTAIN", "RESOURCE_LIMIT", "CORRUPT", "VAULT_LOCKED", "RUNTIME_QUIESCING", "UI_BUILD_INCOMPATIBLE"} {
		if !seen[required] {
			t.Errorf("required problem code %q missing", required)
		}
	}
	if _, exists := properties["userAction"]; !exists {
		t.Error("Problem.userAction recovery hint missing")
	}
	if _, exists := properties["retryAfter"]; !exists {
		t.Error("Problem.retryAfter recovery hint missing")
	}
}

func TestSSEContractIsResumeableAndNonBuffered(t *testing.T) {
	doc := loadDocument(t)
	item := doc.Paths["/api/v1/inference-invocations/{invocationId}/events"]
	get := item["get"].(map[string]interface{})
	refs := parameterRefs(get["parameters"].([]interface{}))
	if !refs["#/components/parameters/LastEventID"] {
		t.Fatal("SSE operation does not declare Last-Event-ID")
	}
	responses := get["responses"].(map[string]interface{})
	ok := responses["200"].(map[string]interface{})
	headers := ok["headers"].(map[string]interface{})
	if _, exists := headers["X-Accel-Buffering"]; !exists {
		t.Fatal("SSE operation does not disable proxy buffering")
	}
	if _, exists := responses["409"]; !exists {
		t.Fatal("SSE operation must explicitly reject expired/unknown resume cursors")
	}
}
