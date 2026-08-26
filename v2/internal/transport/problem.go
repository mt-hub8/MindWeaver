// Package transport defines the small HTTP-facing error contract.
package transport

import (
	"fmt"
	"strings"
)

type ErrorCode string

// RecoveryAction is a stable, content-free UI decision. Retryable means the
// same request may be retried later without first performing this action; it
// does not weaken endpoint-specific idempotency or revision requirements.
type RecoveryAction string

const (
	CodeInvalidArgument    ErrorCode = "INVALID_ARGUMENT"
	CodeUnauthenticated    ErrorCode = "UNAUTHENTICATED"
	CodeForbidden          ErrorCode = "FORBIDDEN"
	CodeNotFound           ErrorCode = "NOT_FOUND"
	CodeConflict           ErrorCode = "CONFLICT"
	CodeResourceLimit      ErrorCode = "RESOURCE_LIMIT"
	CodeServiceUnavailable ErrorCode = "SERVICE_UNAVAILABLE"
	CodeInternal           ErrorCode = "INTERNAL"

	ActionCorrectRequest     RecoveryAction = "correct_request"
	ActionRestartSession     RecoveryAction = "restart_session"
	ActionRefreshState       RecoveryAction = "refresh_state"
	ActionReviewLimits       RecoveryAction = "review_limits"
	ActionRetryLater         RecoveryAction = "retry_later"
	ActionInspectDiagnostics RecoveryAction = "inspect_diagnostics"
)

type Problem struct {
	Type       string         `json:"type"`
	Title      string         `json:"title"`
	Status     int            `json:"status"`
	Detail     string         `json:"detail,omitempty"`
	Instance   string         `json:"instance,omitempty"`
	Code       ErrorCode      `json:"code"`
	RequestID  string         `json:"requestId"`
	Retryable  bool           `json:"retryable"`
	UserAction RecoveryAction `json:"userAction"`
}

type problemSpec struct {
	Status     int
	Title      string
	Retryable  bool
	UserAction RecoveryAction
}

var problemSpecs = map[ErrorCode]problemSpec{
	CodeInvalidArgument:    {Status: 400, Title: "Invalid request", UserAction: ActionCorrectRequest},
	CodeUnauthenticated:    {Status: 401, Title: "Authentication required", UserAction: ActionRestartSession},
	CodeForbidden:          {Status: 403, Title: "Request forbidden", UserAction: ActionRestartSession},
	CodeNotFound:           {Status: 404, Title: "Resource not found", UserAction: ActionRefreshState},
	CodeConflict:           {Status: 409, Title: "Request conflict", UserAction: ActionRefreshState},
	CodeResourceLimit:      {Status: 413, Title: "Resource limit exceeded", UserAction: ActionReviewLimits},
	CodeServiceUnavailable: {Status: 503, Title: "Service unavailable", Retryable: true, UserAction: ActionRetryLater},
	CodeInternal:           {Status: 500, Title: "Internal error", UserAction: ActionInspectDiagnostics},
}

// NewProblem uses a single fixed mapping for status, title and type. Handlers
// may supply safe detail, but cannot create contradictory recovery semantics.
func NewProblem(code ErrorCode, safeDetail, requestID string) Problem {
	spec, ok := problemSpecs[code]
	if !ok {
		code = CodeInternal
		spec = problemSpecs[code]
	}
	return Problem{
		Type:       "https://mindweaver.local/problems/" + strings.ToLower(strings.ReplaceAll(string(code), "_", "-")),
		Title:      spec.Title,
		Status:     spec.Status,
		Detail:     safeDetail,
		Code:       code,
		RequestID:  requestID,
		Retryable:  spec.Retryable,
		UserAction: spec.UserAction,
	}
}

func (p Problem) Validate() error {
	spec, ok := problemSpecs[p.Code]
	if !ok {
		return fmt.Errorf("unknown problem code %q", p.Code)
	}
	wantType := "https://mindweaver.local/problems/" + strings.ToLower(strings.ReplaceAll(string(p.Code), "_", "-"))
	if p.Status != spec.Status || p.Title != spec.Title || p.Type != wantType ||
		p.Retryable != spec.Retryable || p.UserAction != spec.UserAction {
		return fmt.Errorf("problem fields do not match code %q", p.Code)
	}
	if strings.TrimSpace(p.RequestID) == "" {
		return fmt.Errorf("problem requestId is required")
	}
	return nil
}
