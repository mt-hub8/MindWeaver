package transport

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNewProblemUsesFixedSafeMapping(t *testing.T) {
	p := NewProblem(CodeServiceUnavailable, "Try again later.", "req_01")
	if err := p.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if p.Status != 503 || p.Title != "Service unavailable" {
		t.Fatalf("unexpected fixed fields: %#v", p)
	}
	encoded, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"stack", "cause", "credential", "prompt", "userAction", "retryable"} {
		if strings.Contains(strings.ToLower(string(encoded)), strings.ToLower(forbidden)) {
			t.Fatalf("serialized problem leaked or retained %q: %s", forbidden, encoded)
		}
	}
}

func TestUnknownCodeFallsBackToInternal(t *testing.T) {
	p := NewProblem("FUTURE_CODE", "Operation failed.", "req_02")
	if p.Code != CodeInternal || p.Status != 500 {
		t.Fatalf("NewProblem() = %#v", p)
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestProblemRejectsContradictoryOrIncompleteFields(t *testing.T) {
	p := NewProblem(CodeNotFound, "", "req_03")
	p.Status = 500
	if err := p.Validate(); err == nil {
		t.Fatal("Validate() accepted contradictory status")
	}

	p = NewProblem(CodeInternal, "", "")
	if err := p.Validate(); err == nil {
		t.Fatal("Validate() accepted missing requestId")
	}
}
