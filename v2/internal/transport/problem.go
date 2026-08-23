// Package transport defines protocol-facing types shared by HTTP adapters.
package transport

import (
	"fmt"
	"strings"
)

// ErrorCode is a stable, machine-readable error identifier. Clients must branch
// on Code instead of the human-readable Title or Detail fields.
type ErrorCode string

const (
	CodeInvalidArgument               ErrorCode = "INVALID_ARGUMENT"
	CodeValidationFailed              ErrorCode = "VALIDATION_FAILED"
	CodeUnauthenticated               ErrorCode = "UNAUTHENTICATED"
	CodeForbidden                     ErrorCode = "FORBIDDEN"
	CodeCSRFFailed                    ErrorCode = "CSRF_FAILED"
	CodeOriginNotAllowed              ErrorCode = "ORIGIN_NOT_ALLOWED"
	CodeNotFound                      ErrorCode = "NOT_FOUND"
	CodeConflict                      ErrorCode = "CONFLICT"
	CodePreconditionRequired          ErrorCode = "PRECONDITION_REQUIRED"
	CodePreconditionFailed            ErrorCode = "PRECONDITION_FAILED"
	CodeIdempotencyKeyReused          ErrorCode = "IDEMPOTENCY_KEY_REUSED"
	CodeRateLimited                   ErrorCode = "RATE_LIMITED"
	CodeProviderUnavailable           ErrorCode = "PROVIDER_UNAVAILABLE"
	CodeProviderCapabilityUnsupported ErrorCode = "PROVIDER_CAPABILITY_UNSUPPORTED"
	CodeEgressDenied                  ErrorCode = "EGRESS_DENIED"
	CodeInferenceFailed               ErrorCode = "INFERENCE_FAILED"
	CodeContextInsufficient           ErrorCode = "CONTEXT_INSUFFICIENT"
	CodeCitationInvalid               ErrorCode = "CITATION_INVALID"
	CodeOutcomeUncertain              ErrorCode = "OUTCOME_UNCERTAIN"
	CodeResourceLimit                 ErrorCode = "RESOURCE_LIMIT"
	CodeCorrupt                       ErrorCode = "CORRUPT"
	CodeVaultLocked                   ErrorCode = "VAULT_LOCKED"
	CodeRuntimeQuiescing              ErrorCode = "RUNTIME_QUIESCING"
	CodeUIBuildIncompatible           ErrorCode = "UI_BUILD_INCOMPATIBLE"
	CodeInternal                      ErrorCode = "INTERNAL"
	CodeServiceUnavailable            ErrorCode = "SERVICE_UNAVAILABLE"
)

// UserAction is a stable recovery hint for clients. It is deliberately coarser
// than UI copy so transports can remain compatible as the interface evolves.
type UserAction string

const (
	ActionFixRequest       UserAction = "fix_request"
	ActionReauthenticate   UserAction = "reauthenticate"
	ActionRefreshSession   UserAction = "refresh_session"
	ActionRefreshResource  UserAction = "refresh_resource"
	ActionRefreshStatus    UserAction = "refresh_status"
	ActionRetryLater       UserAction = "retry_later"
	ActionChangeProvider   UserAction = "change_provider"
	ActionReviewPolicy     UserAction = "review_policy"
	ActionAddContext       UserAction = "add_context"
	ActionReduceRequest    UserAction = "reduce_request"
	ActionRepairData       UserAction = "repair_data"
	ActionUnlockVault      UserAction = "unlock_vault"
	ActionReloadUI         UserAction = "reload_ui"
	ActionRestartRuntime   UserAction = "restart_runtime"
	ActionUseNewRequestKey UserAction = "use_new_request_key"
)

// Violation describes one invalid request member without echoing its value.
type Violation struct {
	Field   string `json:"field"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// Problem is the only error body exposed by the v1 HTTP API. It follows the
// RFC 9457 problem-details shape and adds stable code and correlation fields.
// Cause, stack traces, provider payloads, prompts, and credentials deliberately
// are not represented here.
type Problem struct {
	Type       string     `json:"type"`
	Title      string     `json:"title"`
	Status     int        `json:"status"`
	Detail     string     `json:"detail,omitempty"`
	Instance   string     `json:"instance,omitempty"`
	Code       ErrorCode  `json:"code"`
	RequestID  string     `json:"requestId"`
	Retryable  bool       `json:"retryable"`
	UserAction UserAction `json:"userAction,omitempty"`
	// RetryAfter is a whole number of seconds. It is present only when the
	// server has a concrete retry window, not merely because Retryable is true.
	RetryAfter int         `json:"retryAfter,omitempty"`
	Violations []Violation `json:"violations,omitempty"`
}

// NewProblem builds a safe client-facing problem. Detail is intentionally
// supplied by the caller rather than derived from an error chain.
func NewProblem(status int, code ErrorCode, title, safeDetail, requestID string) Problem {
	recovery := recoveryByCode[code]
	return Problem{
		Type:       "https://mindweaver.local/problems/" + strings.ToLower(strings.ReplaceAll(string(code), "_", "-")),
		Title:      title,
		Status:     status,
		Detail:     safeDetail,
		Code:       code,
		RequestID:  requestID,
		Retryable:  recovery.Retryable,
		UserAction: recovery.UserAction,
	}
}

// WithRetryAfter attaches a server-provided retry delay without changing the
// stable retry classification. Invalid values are rejected by Validate.
func (p Problem) WithRetryAfter(seconds int) Problem {
	p.RetryAfter = seconds
	return p
}

// Validate rejects malformed problems before an adapter serializes them.
func (p Problem) Validate() error {
	if p.Status < 400 || p.Status > 599 {
		return fmt.Errorf("problem status must be an HTTP error status")
	}
	if !knownErrorCodes[p.Code] {
		return fmt.Errorf("unknown problem code %q", p.Code)
	}
	if strings.TrimSpace(p.Type) == "" || strings.TrimSpace(p.Title) == "" {
		return fmt.Errorf("problem type and title are required")
	}
	if strings.TrimSpace(p.RequestID) == "" {
		return fmt.Errorf("problem requestId is required")
	}
	recovery, known := recoveryByCode[p.Code]
	if !known {
		return fmt.Errorf("problem code %q has no recovery semantics", p.Code)
	}
	if p.Retryable != recovery.Retryable {
		return fmt.Errorf("problem retryable does not match semantics for %q", p.Code)
	}
	if p.UserAction != "" && !knownUserActions[p.UserAction] {
		return fmt.Errorf("unknown problem userAction %q", p.UserAction)
	}
	if p.RetryAfter < 0 || (p.RetryAfter > 0 && !p.Retryable) {
		return fmt.Errorf("problem retryAfter requires a positive retryable error")
	}
	for i, violation := range p.Violations {
		if strings.TrimSpace(violation.Field) == "" || strings.TrimSpace(violation.Rule) == "" {
			return fmt.Errorf("violation %d requires field and rule", i)
		}
	}
	return nil
}

var knownErrorCodes = map[ErrorCode]bool{
	CodeInvalidArgument:               true,
	CodeValidationFailed:              true,
	CodeUnauthenticated:               true,
	CodeForbidden:                     true,
	CodeCSRFFailed:                    true,
	CodeOriginNotAllowed:              true,
	CodeNotFound:                      true,
	CodeConflict:                      true,
	CodePreconditionRequired:          true,
	CodePreconditionFailed:            true,
	CodeIdempotencyKeyReused:          true,
	CodeRateLimited:                   true,
	CodeProviderUnavailable:           true,
	CodeProviderCapabilityUnsupported: true,
	CodeEgressDenied:                  true,
	CodeInferenceFailed:               true,
	CodeContextInsufficient:           true,
	CodeCitationInvalid:               true,
	CodeOutcomeUncertain:              true,
	CodeResourceLimit:                 true,
	CodeCorrupt:                       true,
	CodeVaultLocked:                   true,
	CodeRuntimeQuiescing:              true,
	CodeUIBuildIncompatible:           true,
	CodeInternal:                      true,
	CodeServiceUnavailable:            true,
}

type recoverySemantics struct {
	Retryable  bool
	UserAction UserAction
}

var recoveryByCode = map[ErrorCode]recoverySemantics{
	CodeInvalidArgument:               {UserAction: ActionFixRequest},
	CodeValidationFailed:              {UserAction: ActionFixRequest},
	CodeUnauthenticated:               {UserAction: ActionReauthenticate},
	CodeForbidden:                     {},
	CodeCSRFFailed:                    {UserAction: ActionRefreshSession},
	CodeOriginNotAllowed:              {UserAction: ActionRefreshSession},
	CodeNotFound:                      {},
	CodeConflict:                      {UserAction: ActionRefreshStatus},
	CodePreconditionRequired:          {UserAction: ActionRefreshResource},
	CodePreconditionFailed:            {UserAction: ActionRefreshResource},
	CodeIdempotencyKeyReused:          {UserAction: ActionUseNewRequestKey},
	CodeRateLimited:                   {Retryable: true, UserAction: ActionRetryLater},
	CodeProviderUnavailable:           {Retryable: true, UserAction: ActionRetryLater},
	CodeProviderCapabilityUnsupported: {UserAction: ActionChangeProvider},
	CodeEgressDenied:                  {UserAction: ActionReviewPolicy},
	CodeInferenceFailed:               {UserAction: ActionChangeProvider},
	CodeContextInsufficient:           {UserAction: ActionAddContext},
	CodeCitationInvalid:               {UserAction: ActionAddContext},
	CodeOutcomeUncertain:              {UserAction: ActionRefreshStatus},
	CodeResourceLimit:                 {UserAction: ActionReduceRequest},
	CodeCorrupt:                       {UserAction: ActionRepairData},
	CodeVaultLocked:                   {UserAction: ActionUnlockVault},
	CodeRuntimeQuiescing:              {Retryable: true, UserAction: ActionRetryLater},
	CodeUIBuildIncompatible:           {UserAction: ActionReloadUI},
	CodeInternal:                      {UserAction: ActionRestartRuntime},
	CodeServiceUnavailable:            {Retryable: true, UserAction: ActionRetryLater},
}

var knownUserActions = map[UserAction]bool{
	ActionFixRequest:       true,
	ActionReauthenticate:   true,
	ActionRefreshSession:   true,
	ActionRefreshResource:  true,
	ActionRefreshStatus:    true,
	ActionRetryLater:       true,
	ActionChangeProvider:   true,
	ActionReviewPolicy:     true,
	ActionAddContext:       true,
	ActionReduceRequest:    true,
	ActionRepairData:       true,
	ActionUnlockVault:      true,
	ActionReloadUI:         true,
	ActionRestartRuntime:   true,
	ActionUseNewRequestKey: true,
}
