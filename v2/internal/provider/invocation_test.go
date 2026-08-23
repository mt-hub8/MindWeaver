package provider

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestInferenceInvocationStreamingSuccessLifecycle(t *testing.T) {
	base := time.Date(2026, 8, 23, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	invocation, err := NewInferenceInvocation("inv-1", "offline-fake", "chat-model", CapabilityChat, base)
	if err != nil {
		t.Fatalf("NewInferenceInvocation() error = %v", err)
	}
	if invocation.CreatedAt.Location() != time.UTC {
		t.Fatalf("CreatedAt not canonical UTC: %v", invocation.CreatedAt)
	}

	mustApply(t, &invocation, InvocationTransition{To: InvocationAuthorized, At: base.Add(time.Second)})
	mustApply(t, &invocation, InvocationTransition{To: InvocationRunning, At: base.Add(2 * time.Second)})
	mustApply(t, &invocation, InvocationTransition{To: InvocationStreaming, At: base.Add(3 * time.Second)})
	usage := &InvocationUsage{
		Source:            UsageProviderReported,
		Requests:          1,
		InputTokens:       100,
		OutputTokens:      25,
		TotalTokens:       125,
		CachedInputTokens: 20,
		ReasoningTokens:   5,
		ToolCalls:         1,
	}
	mustApply(t, &invocation, InvocationTransition{To: InvocationSucceeded, At: base.Add(4 * time.Second), Usage: usage})

	if invocation.State != InvocationSucceeded || !invocation.State.IsTerminal() {
		t.Fatalf("state = %q", invocation.State)
	}
	if invocation.Failure != nil || invocation.Refusal != nil || invocation.Cancellation != nil {
		t.Fatal("success contains non-success outcome")
	}
	if invocation.Usage == nil || invocation.Usage.TotalTokens != 125 {
		t.Fatalf("usage = %#v", invocation.Usage)
	}
	if err := invocation.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestInvocationTerminalsAreIrreversible(t *testing.T) {
	base := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		transition InvocationTransition
	}{
		{
			name: "failed",
			transition: InvocationTransition{To: InvocationFailed, At: base.Add(time.Second), Failure: &InvocationFailure{
				Class: FailurePolicyDenied, Code: "EGRESS_DENIED",
			}},
		},
		{
			name: "cancelled",
			transition: InvocationTransition{To: InvocationCancelled, At: base.Add(time.Second), Cancellation: &InvocationCancellation{
				Origin: CancellationUser, Code: "USER_CANCELLED",
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invocation := mustInvocation(t, base)
			mustApply(t, &invocation, test.transition)
			before := invocation
			if err := invocation.Apply(InvocationTransition{To: InvocationAuthorized, At: base.Add(2 * time.Second)}); err == nil {
				t.Fatal("terminal state reopened")
			}
			if !reflect.DeepEqual(invocation, before) {
				t.Fatal("failed terminal transition mutated receiver")
			}
		})
	}

	invocation := runningInvocation(t, base)
	mustApply(t, &invocation, InvocationTransition{To: InvocationRefused, At: base.Add(3 * time.Second), Refusal: &InvocationRefusal{
		Class: RefusalSafety, Code: "MODEL_SAFETY_REFUSAL",
	}})
	if err := invocation.Apply(InvocationTransition{To: InvocationFailed, At: base.Add(4 * time.Second), Failure: &InvocationFailure{Class: FailureProvider, Code: "LATE_ERROR"}}); err == nil {
		t.Fatal("refused terminal state reopened")
	}
}

func TestFailureRefusalAndCancellationAreDistinct(t *testing.T) {
	base := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)

	failed := mustInvocation(t, base)
	mustApply(t, &failed, InvocationTransition{To: InvocationFailed, At: base.Add(time.Second), Failure: &InvocationFailure{
		Class: FailurePolicyDenied, Code: "EGRESS_DEFAULT_DENY",
	}})
	if failed.State != InvocationFailed || failed.Failure == nil || failed.Refusal != nil || failed.Cancellation != nil {
		t.Fatalf("failed outcome = %#v", failed)
	}

	refused := runningInvocation(t, base)
	mustApply(t, &refused, InvocationTransition{To: InvocationRefused, At: base.Add(3 * time.Second), Refusal: &InvocationRefusal{
		Class: RefusalContentPolicy, Code: "CONTENT_POLICY",
	}})
	if refused.State != InvocationRefused || refused.Failure != nil || refused.Refusal == nil || refused.Cancellation != nil {
		t.Fatalf("refused outcome = %#v", refused)
	}

	cancelled := runningInvocation(t, base)
	mustApply(t, &cancelled, InvocationTransition{To: InvocationCancelRequested, At: base.Add(3 * time.Second)})
	mustApply(t, &cancelled, InvocationTransition{To: InvocationCancelled, At: base.Add(4 * time.Second), Cancellation: &InvocationCancellation{
		Origin: CancellationUser, Code: "USER_CANCELLED",
	}})
	if cancelled.State != InvocationCancelled || cancelled.Failure != nil || cancelled.Refusal != nil || cancelled.Cancellation == nil {
		t.Fatalf("cancelled outcome = %#v", cancelled)
	}

	for state, invocation := range map[string]InferenceInvocation{"failed": failed, "refused": refused, "cancelled": cancelled} {
		encoded, err := json.Marshal(invocation)
		if err != nil {
			t.Fatalf("json.Marshal(%s) error = %v", state, err)
		}
		if !strings.Contains(string(encoded), `"state":"`+state+`"`) {
			t.Errorf("serialized state %s missing: %s", state, encoded)
		}
	}
}

func TestInvocationApplyRejectsInvalidOutcomeWithoutMutation(t *testing.T) {
	base := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)
	invocation := runningInvocation(t, base)
	before := invocation

	err := invocation.Apply(InvocationTransition{
		To:      InvocationRefused,
		At:      base.Add(3 * time.Second),
		Failure: &InvocationFailure{Class: FailureProvider, Code: "WRONG_PAYLOAD"},
	})
	if err == nil {
		t.Fatal("refusal with failure payload accepted")
	}
	if !reflect.DeepEqual(invocation, before) {
		t.Fatal("invalid outcome mutated receiver")
	}
}

func TestInvocationRejectsRetryablePermanentFailure(t *testing.T) {
	base := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)
	invocation := mustInvocation(t, base)
	err := invocation.Apply(InvocationTransition{To: InvocationFailed, At: base.Add(time.Second), Failure: &InvocationFailure{
		Class: FailureAuthentication, Code: "INVALID_CREDENTIAL", Retryable: true, HTTPStatus: 401,
	}})
	if err == nil {
		t.Fatal("retryable authentication failure accepted")
	}
	if invocation.State != InvocationCreated {
		t.Fatalf("invalid failure mutated state to %q", invocation.State)
	}
}

func TestInvocationRejectsInvalidUsageAndChronology(t *testing.T) {
	base := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)
	invocation := runningInvocation(t, base)
	invalidUsage := &InvocationUsage{
		Source: UsageProviderReported, Requests: 1, InputTokens: 10, OutputTokens: 5, TotalTokens: 99,
	}
	if err := invocation.Apply(InvocationTransition{To: InvocationSucceeded, At: base.Add(3 * time.Second), Usage: invalidUsage}); err == nil {
		t.Fatal("inconsistent usage accepted")
	}
	if invocation.State != InvocationRunning {
		t.Fatal("invalid usage mutated invocation")
	}

	if err := invocation.Apply(InvocationTransition{To: InvocationSucceeded, At: base.Add(time.Second)}); err == nil {
		t.Fatal("backdated completion accepted")
	}
}

func TestEmbeddingInvocationRejectsChatOnlyUsage(t *testing.T) {
	base := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)
	invocation, err := NewInferenceInvocation("inv-embed", "offline-fake", "embedding-model", CapabilityEmbedding, base)
	if err != nil {
		t.Fatalf("NewInferenceInvocation() error = %v", err)
	}
	mustApply(t, &invocation, InvocationTransition{To: InvocationAuthorized, At: base.Add(time.Second)})
	mustApply(t, &invocation, InvocationTransition{To: InvocationRunning, At: base.Add(2 * time.Second)})
	usage := &InvocationUsage{Source: UsageProviderReported, Requests: 1, InputTokens: 10, OutputTokens: 1, TotalTokens: 11, EmbeddingItems: 2}
	if err := invocation.Apply(InvocationTransition{To: InvocationSucceeded, At: base.Add(3 * time.Second), Usage: usage}); err == nil {
		t.Fatal("embedding invocation accepted output-token usage")
	}
}

func TestCanTransitionEnumeratesExpectedGraph(t *testing.T) {
	if !CanTransition(InvocationRunning, InvocationStreaming) || !CanTransition(InvocationCancelRequested, InvocationSucceeded) {
		t.Fatal("expected transition rejected")
	}
	for _, terminal := range []InvocationState{InvocationSucceeded, InvocationFailed, InvocationRefused, InvocationCancelled} {
		for _, candidate := range []InvocationState{InvocationCreated, InvocationAuthorized, InvocationRunning, InvocationStreaming, InvocationCancelRequested, InvocationSucceeded, InvocationFailed, InvocationRefused, InvocationCancelled} {
			if CanTransition(terminal, candidate) {
				t.Errorf("terminal %q transitions to %q", terminal, candidate)
			}
		}
	}
	if CanTransition(InvocationRunning, InvocationRunning) {
		t.Fatal("self-transition accepted")
	}
}

func mustInvocation(t *testing.T, base time.Time) InferenceInvocation {
	t.Helper()
	invocation, err := NewInferenceInvocation("inv-1", "offline-fake", "chat-model", CapabilityChat, base)
	if err != nil {
		t.Fatalf("NewInferenceInvocation() error = %v", err)
	}
	return invocation
}

func runningInvocation(t *testing.T, base time.Time) InferenceInvocation {
	t.Helper()
	invocation := mustInvocation(t, base)
	mustApply(t, &invocation, InvocationTransition{To: InvocationAuthorized, At: base.Add(time.Second)})
	mustApply(t, &invocation, InvocationTransition{To: InvocationRunning, At: base.Add(2 * time.Second)})
	return invocation
}

func mustApply(t *testing.T, invocation *InferenceInvocation, transition InvocationTransition) {
	t.Helper()
	if err := invocation.Apply(transition); err != nil {
		t.Fatalf("Apply(%q) error = %v", transition.To, err)
	}
}
