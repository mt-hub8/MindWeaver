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
	if p.UserAction != ActionRetryLater {
		t.Fatalf("userAction = %q, want %q", p.UserAction, ActionRetryLater)
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

func TestRetryabilityComesFromStableErrorSemantics(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		code      ErrorCode
		retryable bool
		action    UserAction
	}{
		{name: "provider bad gateway is retryable", status: http.StatusBadGateway, code: CodeProviderUnavailable, retryable: true, action: ActionRetryLater},
		{name: "uncertain outcome is not blindly retried", status: http.StatusServiceUnavailable, code: CodeOutcomeUncertain, retryable: false, action: ActionRefreshStatus},
		{name: "resource limit requires smaller input", status: http.StatusTooManyRequests, code: CodeResourceLimit, retryable: false, action: ActionReduceRequest},
		{name: "quiescing runtime can be retried later", status: http.StatusServiceUnavailable, code: CodeRuntimeQuiescing, retryable: true, action: ActionRetryLater},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			problem := NewProblem(tt.status, tt.code, "title", "detail", "req_01")
			if problem.Retryable != tt.retryable {
				t.Fatalf("Retryable = %v, want %v", problem.Retryable, tt.retryable)
			}
			if problem.UserAction != tt.action {
				t.Fatalf("UserAction = %q, want %q", problem.UserAction, tt.action)
			}
			if err := problem.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestRetryAfterRequiresRetryableSemantics(t *testing.T) {
	retryable := NewProblem(http.StatusTooManyRequests, CodeRateLimited, "Limited", "Wait.", "req_01").WithRetryAfter(5)
	if err := retryable.Validate(); err != nil {
		t.Fatalf("retryable problem rejected: %v", err)
	}

	nonRetryable := NewProblem(http.StatusServiceUnavailable, CodeOutcomeUncertain, "Unknown", "Refresh status.", "req_02").WithRetryAfter(5)
	if err := nonRetryable.Validate(); err == nil {
		t.Fatal("non-retryable problem accepted retryAfter")
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
