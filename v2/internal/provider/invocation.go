package provider

import (
	"errors"
	"time"
)

// InvocationSchemaVersion identifies the serialized invocation state contract.
const InvocationSchemaVersion = "mindweaver.inference-invocation/v1"

// InvocationState is the durable lifecycle state of one provider call.
type InvocationState string

const (
	InvocationCreated         InvocationState = "created"
	InvocationAuthorized      InvocationState = "authorized"
	InvocationRunning         InvocationState = "running"
	InvocationStreaming       InvocationState = "streaming"
	InvocationCancelRequested InvocationState = "cancel_requested"
	InvocationSucceeded       InvocationState = "succeeded"
	InvocationFailed          InvocationState = "failed"
	InvocationRefused         InvocationState = "refused"
	InvocationCancelled       InvocationState = "cancelled"
)

// IsTerminal reports whether no further transition is legal.
func (s InvocationState) IsTerminal() bool {
	return s == InvocationSucceeded || s == InvocationFailed || s == InvocationRefused || s == InvocationCancelled
}

// CanTransition defines the complete invocation state graph. Self-transitions
// are rejected so event replay cannot silently mutate timing or outcomes.
func CanTransition(from, to InvocationState) bool {
	switch from {
	case InvocationCreated:
		return to == InvocationAuthorized || to == InvocationFailed || to == InvocationCancelled
	case InvocationAuthorized:
		return to == InvocationRunning || to == InvocationFailed || to == InvocationCancelled
	case InvocationRunning:
		return to == InvocationStreaming || to == InvocationCancelRequested || to == InvocationSucceeded || to == InvocationFailed || to == InvocationRefused || to == InvocationCancelled
	case InvocationStreaming:
		return to == InvocationCancelRequested || to == InvocationSucceeded || to == InvocationFailed || to == InvocationRefused || to == InvocationCancelled
	case InvocationCancelRequested:
		// Completion/refusal can win a race with cooperative cancellation.
		return to == InvocationSucceeded || to == InvocationFailed || to == InvocationRefused || to == InvocationCancelled
	default:
		return false
	}
}

// FailureClass describes an execution failure. Refusal and cancellation are
// deliberately represented by separate types and terminal states.
type FailureClass string

const (
	FailurePolicyDenied   FailureClass = "policy_denied"
	FailureInvalidRequest FailureClass = "invalid_request"
	FailureUnsupported    FailureClass = "unsupported"
	FailureAuthentication FailureClass = "authentication"
	FailureAuthorization  FailureClass = "authorization"
	FailureRateLimited    FailureClass = "rate_limited"
	FailureTimeout        FailureClass = "timeout"
	FailureUnavailable    FailureClass = "unavailable"
	FailureProvider       FailureClass = "provider_error"
	FailureProtocol       FailureClass = "protocol_error"
	FailureInternal       FailureClass = "internal"
)

// InvocationFailure contains only classified, safe metadata. Raw provider
// errors and response bodies must be redacted and stored outside this contract.
type InvocationFailure struct {
	Class      FailureClass `json:"class"`
	Code       string       `json:"code"`
	Retryable  bool         `json:"retryable"`
	HTTPStatus uint16       `json:"http_status,omitempty"`
}

// RefusalClass describes a successful provider interaction that intentionally
// produced no answer. It is not an infrastructure failure.
type RefusalClass string

const (
	RefusalSafety             RefusalClass = "safety"
	RefusalContentPolicy      RefusalClass = "content_policy"
	RefusalUnsupportedContent RefusalClass = "unsupported_content"
	RefusalModelDeclined      RefusalClass = "model_declined"
)

type InvocationRefusal struct {
	Class RefusalClass `json:"class"`
	Code  string       `json:"code"`
}

// CancellationOrigin records who ended cooperative execution.
type CancellationOrigin string

const (
	CancellationUser     CancellationOrigin = "user"
	CancellationSystem   CancellationOrigin = "system"
	CancellationDeadline CancellationOrigin = "deadline"
	CancellationProvider CancellationOrigin = "provider"
)

type InvocationCancellation struct {
	Origin CancellationOrigin `json:"origin"`
	Code   string             `json:"code"`
}

// UsageSource distinguishes provider accounting from a local estimate.
type UsageSource string

const (
	UsageProviderReported UsageSource = "provider_reported"
	UsageEstimated        UsageSource = "estimated"
)

// InvocationUsage is a non-negative accounting summary for exactly one
// provider request. Cached tokens are included in InputTokens and reasoning
// tokens are included in OutputTokens.
type InvocationUsage struct {
	Source            UsageSource `json:"source"`
	Requests          uint32      `json:"requests"`
	InputTokens       uint64      `json:"input_tokens"`
	OutputTokens      uint64      `json:"output_tokens"`
	TotalTokens       uint64      `json:"total_tokens"`
	CachedInputTokens uint64      `json:"cached_input_tokens,omitempty"`
	ReasoningTokens   uint64      `json:"reasoning_tokens,omitempty"`
	EmbeddingItems    uint32      `json:"embedding_items,omitempty"`
	ToolCalls         uint32      `json:"tool_calls,omitempty"`
}

// Validate checks accounting consistency and overflow.
func (u InvocationUsage) Validate() error {
	if u.Source != UsageProviderReported && u.Source != UsageEstimated {
		return errors.New("invalid invocation usage: source")
	}
	if u.Requests != 1 {
		return errors.New("invalid invocation usage: requests")
	}
	if u.InputTokens > ^uint64(0)-u.OutputTokens || u.TotalTokens != u.InputTokens+u.OutputTokens {
		return errors.New("invalid invocation usage: total_tokens")
	}
	if u.CachedInputTokens > u.InputTokens {
		return errors.New("invalid invocation usage: cached_input_tokens")
	}
	if u.ReasoningTokens > u.OutputTokens {
		return errors.New("invalid invocation usage: reasoning_tokens")
	}
	return nil
}

// InferenceInvocation is the durable, content-free audit record for one call.
// It intentionally excludes credentials, headers, prompts, generated content,
// raw provider payloads, and raw error text.
type InferenceInvocation struct {
	SchemaVersion string          `json:"schema_version"`
	ID            string          `json:"id"`
	ProviderID    string          `json:"provider_id"`
	ModelID       string          `json:"model_id"`
	Capability    Capability      `json:"capability"`
	State         InvocationState `json:"state"`

	Failure      *InvocationFailure      `json:"failure,omitempty"`
	Refusal      *InvocationRefusal      `json:"refusal,omitempty"`
	Cancellation *InvocationCancellation `json:"cancellation,omitempty"`
	Usage        *InvocationUsage        `json:"usage,omitempty"`

	CreatedAt         time.Time  `json:"created_at"`
	AuthorizedAt      *time.Time `json:"authorized_at,omitempty"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	FirstOutputAt     *time.Time `json:"first_output_at,omitempty"`
	CancelRequestedAt *time.Time `json:"cancel_requested_at,omitempty"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// InvocationTransition is applied atomically. Exactly one outcome payload is
// required for failed, refused, or cancelled terminal states.
type InvocationTransition struct {
	To           InvocationState
	At           time.Time
	Failure      *InvocationFailure
	Refusal      *InvocationRefusal
	Cancellation *InvocationCancellation
	Usage        *InvocationUsage
}

// InvocationError is safe to expose because it contains only schema field and
// stable error code.
type InvocationError struct {
	Field string
	Code  string
}

func (e *InvocationError) Error() string {
	return "invalid inference invocation " + e.Field + ": " + e.Code
}

// NewInferenceInvocation creates the only valid initial state.
func NewInferenceInvocation(id, providerID, modelID string, capability Capability, createdAt time.Time) (InferenceInvocation, error) {
	invocation := InferenceInvocation{
		SchemaVersion: InvocationSchemaVersion,
		ID:            id,
		ProviderID:    providerID,
		ModelID:       modelID,
		Capability:    capability,
		State:         InvocationCreated,
		CreatedAt:     canonicalTime(createdAt),
		UpdatedAt:     canonicalTime(createdAt),
	}
	if err := invocation.Validate(); err != nil {
		return InferenceInvocation{}, err
	}
	return invocation, nil
}

// Apply validates and commits one legal transition. The receiver is unchanged
// on error, and terminal states can never be reopened.
func (i *InferenceInvocation) Apply(transition InvocationTransition) error {
	if i == nil {
		return &InvocationError{Field: "invocation", Code: "nil"}
	}
	if err := i.Validate(); err != nil {
		return err
	}
	if !CanTransition(i.State, transition.To) {
		return &InvocationError{Field: "state", Code: "illegal_transition"}
	}
	if transition.At.IsZero() {
		return &InvocationError{Field: "at", Code: "required"}
	}
	at := canonicalTime(transition.At)
	if at.Before(i.UpdatedAt) {
		return &InvocationError{Field: "at", Code: "before_current_state"}
	}
	if err := validateTransitionOutcome(transition); err != nil {
		return err
	}

	candidate := *i
	candidate.State = transition.To
	candidate.UpdatedAt = at
	switch transition.To {
	case InvocationAuthorized:
		candidate.AuthorizedAt = timePointer(at)
	case InvocationRunning:
		candidate.StartedAt = timePointer(at)
	case InvocationStreaming:
		candidate.FirstOutputAt = timePointer(at)
	case InvocationCancelRequested:
		candidate.CancelRequestedAt = timePointer(at)
	case InvocationSucceeded, InvocationFailed, InvocationRefused, InvocationCancelled:
		candidate.CompletedAt = timePointer(at)
		candidate.Failure = cloneFailure(transition.Failure)
		candidate.Refusal = cloneRefusal(transition.Refusal)
		candidate.Cancellation = cloneCancellation(transition.Cancellation)
		candidate.Usage = cloneUsage(transition.Usage)
	}
	if err := candidate.Validate(); err != nil {
		return err
	}
	*i = candidate
	return nil
}

// Validate checks identity, state/outcome semantics, accounting, and the full
// timestamp partial order.
func (i InferenceInvocation) Validate() error {
	if i.SchemaVersion != InvocationSchemaVersion {
		return &InvocationError{Field: "schema_version", Code: "unsupported_version"}
	}
	if err := validateIdentifier(i.ID, 128); err != nil {
		return &InvocationError{Field: "id", Code: "invalid_identifier"}
	}
	if err := validateIdentifier(i.ProviderID, 128); err != nil {
		return &InvocationError{Field: "provider_id", Code: "invalid_identifier"}
	}
	if err := validateIdentifier(i.ModelID, 256); err != nil {
		return &InvocationError{Field: "model_id", Code: "invalid_identifier"}
	}
	if i.Capability != CapabilityChat && i.Capability != CapabilityEmbedding {
		return &InvocationError{Field: "capability", Code: "invalid_primary_capability"}
	}
	if !knownInvocationState(i.State) {
		return &InvocationError{Field: "state", Code: "unknown"}
	}
	if i.CreatedAt.IsZero() || i.UpdatedAt.IsZero() || i.UpdatedAt.Before(i.CreatedAt) {
		return &InvocationError{Field: "timestamps", Code: "invalid_bounds"}
	}
	if err := validateInvocationTimes(i); err != nil {
		return err
	}
	if err := validateInvocationOutcome(i); err != nil {
		return err
	}
	if i.Usage != nil {
		if err := i.Usage.Validate(); err != nil {
			return &InvocationError{Field: "usage", Code: "invalid"}
		}
		if i.Capability == CapabilityChat && i.Usage.EmbeddingItems != 0 {
			return &InvocationError{Field: "usage.embedding_items", Code: "not_applicable"}
		}
		if i.Capability == CapabilityEmbedding && (i.Usage.OutputTokens != 0 || i.Usage.ReasoningTokens != 0 || i.Usage.ToolCalls != 0) {
			return &InvocationError{Field: "usage", Code: "chat_usage_on_embedding"}
		}
	}
	return nil
}

func validateTransitionOutcome(transition InvocationTransition) error {
	hasFailure := transition.Failure != nil
	hasRefusal := transition.Refusal != nil
	hasCancellation := transition.Cancellation != nil
	switch transition.To {
	case InvocationFailed:
		if !hasFailure || hasRefusal || hasCancellation {
			return &InvocationError{Field: "failure", Code: "exclusive_payload_required"}
		}
	case InvocationRefused:
		if hasFailure || !hasRefusal || hasCancellation {
			return &InvocationError{Field: "refusal", Code: "exclusive_payload_required"}
		}
	case InvocationCancelled:
		if hasFailure || hasRefusal || !hasCancellation {
			return &InvocationError{Field: "cancellation", Code: "exclusive_payload_required"}
		}
	case InvocationSucceeded:
		if hasFailure || hasRefusal || hasCancellation {
			return &InvocationError{Field: "outcome", Code: "unexpected_payload"}
		}
	default:
		if hasFailure || hasRefusal || hasCancellation || transition.Usage != nil {
			return &InvocationError{Field: "outcome", Code: "terminal_payload_on_nonterminal_state"}
		}
	}
	return nil
}

func validateInvocationOutcome(i InferenceInvocation) error {
	if !i.State.IsTerminal() {
		if i.Failure != nil || i.Refusal != nil || i.Cancellation != nil || i.Usage != nil || i.CompletedAt != nil {
			return &InvocationError{Field: "outcome", Code: "present_before_terminal_state"}
		}
		return nil
	}
	if i.CompletedAt == nil {
		return &InvocationError{Field: "completed_at", Code: "required"}
	}
	switch i.State {
	case InvocationSucceeded:
		if i.Failure != nil || i.Refusal != nil || i.Cancellation != nil {
			return &InvocationError{Field: "outcome", Code: "unexpected_payload"}
		}
	case InvocationFailed:
		if i.Failure == nil || i.Refusal != nil || i.Cancellation != nil || !validFailure(*i.Failure) {
			return &InvocationError{Field: "failure", Code: "invalid_or_nonexclusive"}
		}
	case InvocationRefused:
		if i.Failure != nil || i.Refusal == nil || i.Cancellation != nil || !validRefusal(*i.Refusal) {
			return &InvocationError{Field: "refusal", Code: "invalid_or_nonexclusive"}
		}
	case InvocationCancelled:
		if i.Failure != nil || i.Refusal != nil || i.Cancellation == nil || !validCancellation(*i.Cancellation) {
			return &InvocationError{Field: "cancellation", Code: "invalid_or_nonexclusive"}
		}
	}
	return nil
}

func validateInvocationTimes(i InferenceInvocation) error {
	values := []*time.Time{i.AuthorizedAt, i.StartedAt, i.FirstOutputAt, i.CancelRequestedAt, i.CompletedAt}
	for _, value := range values {
		if value != nil && (value.IsZero() || value.Before(i.CreatedAt) || value.After(i.UpdatedAt)) {
			return &InvocationError{Field: "timestamps", Code: "out_of_order"}
		}
	}
	if i.StartedAt != nil && (i.AuthorizedAt == nil || i.StartedAt.Before(*i.AuthorizedAt)) {
		return &InvocationError{Field: "started_at", Code: "before_authorized"}
	}
	if i.FirstOutputAt != nil && (i.StartedAt == nil || i.FirstOutputAt.Before(*i.StartedAt)) {
		return &InvocationError{Field: "first_output_at", Code: "before_started"}
	}
	if i.CancelRequestedAt != nil && (i.StartedAt == nil || i.CancelRequestedAt.Before(*i.StartedAt)) {
		return &InvocationError{Field: "cancel_requested_at", Code: "before_started"}
	}
	if i.CompletedAt != nil {
		latest := i.CreatedAt
		for _, value := range []*time.Time{i.AuthorizedAt, i.StartedAt, i.FirstOutputAt, i.CancelRequestedAt} {
			if value != nil && value.After(latest) {
				latest = *value
			}
		}
		if i.CompletedAt.Before(latest) {
			return &InvocationError{Field: "completed_at", Code: "before_prior_event"}
		}
	}

	var expectedLatest time.Time
	switch i.State {
	case InvocationCreated:
		expectedLatest = i.CreatedAt
		if i.AuthorizedAt != nil || i.StartedAt != nil || i.FirstOutputAt != nil || i.CancelRequestedAt != nil || i.CompletedAt != nil {
			return &InvocationError{Field: "timestamps", Code: "unexpected_for_created"}
		}
	case InvocationAuthorized:
		if i.AuthorizedAt == nil || i.StartedAt != nil || i.FirstOutputAt != nil || i.CancelRequestedAt != nil || i.CompletedAt != nil {
			return &InvocationError{Field: "timestamps", Code: "invalid_for_authorized"}
		}
		expectedLatest = *i.AuthorizedAt
	case InvocationRunning:
		if i.AuthorizedAt == nil || i.StartedAt == nil || i.FirstOutputAt != nil || i.CancelRequestedAt != nil || i.CompletedAt != nil {
			return &InvocationError{Field: "timestamps", Code: "invalid_for_running"}
		}
		expectedLatest = *i.StartedAt
	case InvocationStreaming:
		if i.AuthorizedAt == nil || i.StartedAt == nil || i.FirstOutputAt == nil || i.CancelRequestedAt != nil || i.CompletedAt != nil {
			return &InvocationError{Field: "timestamps", Code: "invalid_for_streaming"}
		}
		expectedLatest = *i.FirstOutputAt
	case InvocationCancelRequested:
		if i.AuthorizedAt == nil || i.StartedAt == nil || i.CancelRequestedAt == nil || i.CompletedAt != nil {
			return &InvocationError{Field: "timestamps", Code: "invalid_for_cancel_requested"}
		}
		expectedLatest = *i.CancelRequestedAt
	case InvocationSucceeded, InvocationRefused:
		if i.AuthorizedAt == nil || i.StartedAt == nil || i.CompletedAt == nil {
			return &InvocationError{Field: "timestamps", Code: "execution_timestamps_required"}
		}
		expectedLatest = *i.CompletedAt
	case InvocationFailed, InvocationCancelled:
		if i.CompletedAt == nil {
			return &InvocationError{Field: "completed_at", Code: "required"}
		}
		expectedLatest = *i.CompletedAt
	}
	if !i.UpdatedAt.Equal(expectedLatest) {
		return &InvocationError{Field: "updated_at", Code: "not_latest_event"}
	}
	return nil
}

func validFailure(failure InvocationFailure) bool {
	switch failure.Class {
	case FailurePolicyDenied, FailureInvalidRequest, FailureUnsupported, FailureAuthentication, FailureAuthorization, FailureRateLimited, FailureTimeout, FailureUnavailable, FailureProvider, FailureProtocol, FailureInternal:
	default:
		return false
	}
	if validateSafeCode(failure.Code) != nil || (failure.HTTPStatus != 0 && (failure.HTTPStatus < 400 || failure.HTTPStatus > 599)) {
		return false
	}
	if failure.Retryable {
		switch failure.Class {
		case FailurePolicyDenied, FailureInvalidRequest, FailureUnsupported, FailureAuthentication, FailureAuthorization:
			return false
		}
	}
	return true
}

func validRefusal(refusal InvocationRefusal) bool {
	switch refusal.Class {
	case RefusalSafety, RefusalContentPolicy, RefusalUnsupportedContent, RefusalModelDeclined:
		return validateSafeCode(refusal.Code) == nil
	default:
		return false
	}
}

func validCancellation(cancellation InvocationCancellation) bool {
	switch cancellation.Origin {
	case CancellationUser, CancellationSystem, CancellationDeadline, CancellationProvider:
		return validateSafeCode(cancellation.Code) == nil
	default:
		return false
	}
}

func validateSafeCode(value string) error {
	if value == "" || len(value) > 96 {
		return errors.New("invalid code")
	}
	for _, character := range value {
		if !((character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '_' || character == '-' || character == '.' || character == ':') {
			return errors.New("invalid code")
		}
	}
	return nil
}

func knownInvocationState(state InvocationState) bool {
	switch state {
	case InvocationCreated, InvocationAuthorized, InvocationRunning, InvocationStreaming, InvocationCancelRequested, InvocationSucceeded, InvocationFailed, InvocationRefused, InvocationCancelled:
		return true
	default:
		return false
	}
}

func canonicalTime(value time.Time) time.Time { return value.UTC().Round(0) }

func timePointer(value time.Time) *time.Time {
	copy := value
	return &copy
}

func cloneFailure(value *InvocationFailure) *InvocationFailure {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneRefusal(value *InvocationRefusal) *InvocationRefusal {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneCancellation(value *InvocationCancellation) *InvocationCancellation {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneUsage(value *InvocationUsage) *InvocationUsage {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
