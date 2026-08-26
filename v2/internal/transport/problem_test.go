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
	if !p.Retryable || p.UserAction != ActionRetryLater {
		t.Fatalf("unexpected recovery fields: %#v", p)
	}
	encoded, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"stack", "cause", "credential", "prompt"} {
		if strings.Contains(strings.ToLower(string(encoded)), strings.ToLower(forbidden)) {
			t.Fatalf("serialized problem leaked or retained %q: %s", forbidden, encoded)
		}
	}
}

func TestProblemRecoveryMappingIsStableAndConservative(t *testing.T) {
	want := map[ErrorCode]struct {
		retryable bool
		action    RecoveryAction
	}{
		CodeInvalidArgument:    {false, ActionCorrectRequest},
		CodeUnauthenticated:    {false, ActionRestartSession},
		CodeForbidden:          {false, ActionRestartSession},
		CodeNotFound:           {false, ActionRefreshState},
		CodeConflict:           {false, ActionRefreshState},
		CodeResourceLimit:      {false, ActionReviewLimits},
		CodeServiceUnavailable: {true, ActionRetryLater},
		CodeInternal:           {false, ActionInspectDiagnostics},
	}
	for code, expected := range want {
		problem := NewProblem(code, "safe", "request")
		if problem.Retryable != expected.retryable || problem.UserAction != expected.action {
			t.Fatalf("recovery mapping for %s = %t/%q, want %t/%q",
				code, problem.Retryable, problem.UserAction, expected.retryable, expected.action)
		}
		if err := problem.Validate(); err != nil {
			t.Fatalf("Validate(%s): %v", code, err)
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

	p = NewProblem(CodeConflict, "", "req_04")
	p.Retryable = true
	if err := p.Validate(); err == nil {
		t.Fatal("Validate() accepted contradictory retryability")
	}

	p = NewProblem(CodeConflict, "", "req_05")
	p.UserAction = ActionRetryLater
	if err := p.Validate(); err == nil {
		t.Fatal("Validate() accepted contradictory user action")
	}
}
