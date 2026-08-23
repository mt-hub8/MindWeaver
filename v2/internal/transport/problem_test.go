package transport

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestNewProblemProducesStableSafeShape(t *testing.T) {
	p := NewProblem(http.StatusServiceUnavailable, CodeProviderUnavailable, "Provider unavailable", "Try again later.", "req_01")
	if err := p.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !p.Retryable {
		t.Fatal("service unavailable must be retryable")
	}

	encoded, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"stack", "cause", "credential", "prompt"} {
		if strings.Contains(strings.ToLower(string(encoded)), forbidden) {
			t.Fatalf("serialized problem leaked forbidden field %q: %s", forbidden, encoded)
		}
	}
}

func TestProblemValidation(t *testing.T) {
	tests := []struct {
		name    string
		problem Problem
	}{
		{name: "non error status", problem: Problem{Status: 200, Code: CodeInternal, Type: "x", Title: "x", RequestID: "r"}},
		{name: "unknown code", problem: Problem{Status: 500, Code: "BOOM", Type: "x", Title: "x", RequestID: "r"}},
		{name: "missing request id", problem: Problem{Status: 500, Code: CodeInternal, Type: "x", Title: "x"}},
		{name: "invalid violation", problem: Problem{Status: 400, Code: CodeValidationFailed, Type: "x", Title: "x", RequestID: "r", Violations: []Violation{{Field: "query"}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.problem.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want error")
			}
		})
	}
}
