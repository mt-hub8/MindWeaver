// Package transport defines protocol-facing types shared by HTTP adapters.
package transport

import (
	"fmt"
	"net/http"
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
	CodeInternal                      ErrorCode = "INTERNAL"
	CodeServiceUnavailable            ErrorCode = "SERVICE_UNAVAILABLE"
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
	Type       string      `json:"type"`
	Title      string      `json:"title"`
	Status     int         `json:"status"`
	Detail     string      `json:"detail,omitempty"`
	Instance   string      `json:"instance,omitempty"`
	Code       ErrorCode   `json:"code"`
	RequestID  string      `json:"requestId"`
	Retryable  bool        `json:"retryable"`
	Violations []Violation `json:"violations,omitempty"`
}

// NewProblem builds a safe client-facing problem. Detail is intentionally
// supplied by the caller rather than derived from an error chain.
func NewProblem(status int, code ErrorCode, title, safeDetail, requestID string) Problem {
	return Problem{
		Type:      "https://mindweaver.local/problems/" + strings.ToLower(strings.ReplaceAll(string(code), "_", "-")),
		Title:     title,
		Status:    status,
		Detail:    safeDetail,
		Code:      code,
		RequestID: requestID,
		Retryable: status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable,
	}
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
	CodeInternal:                      true,
	CodeServiceUnavailable:            true,
}
