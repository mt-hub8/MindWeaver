// Package transport defines the small HTTP-facing error contract.
package transport

import (
	"fmt"
	"strings"
)

type ErrorCode string

const (
	CodeInvalidArgument    ErrorCode = "INVALID_ARGUMENT"
	CodeUnauthenticated    ErrorCode = "UNAUTHENTICATED"
	CodeForbidden          ErrorCode = "FORBIDDEN"
	CodeNotFound           ErrorCode = "NOT_FOUND"
	CodeConflict           ErrorCode = "CONFLICT"
	CodeResourceLimit      ErrorCode = "RESOURCE_LIMIT"
	CodeServiceUnavailable ErrorCode = "SERVICE_UNAVAILABLE"
	CodeInternal           ErrorCode = "INTERNAL"
)

type Problem struct {
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Status    int       `json:"status"`
	Detail    string    `json:"detail,omitempty"`
	Instance  string    `json:"instance,omitempty"`
	Code      ErrorCode `json:"code"`
	RequestID string    `json:"requestId"`
}

type problemSpec struct {
	Status int
	Title  string
}

var problemSpecs = map[ErrorCode]problemSpec{
	CodeInvalidArgument:    {Status: 400, Title: "Invalid request"},
	CodeUnauthenticated:    {Status: 401, Title: "Authentication required"},
	CodeForbidden:          {Status: 403, Title: "Request forbidden"},
	CodeNotFound:           {Status: 404, Title: "Resource not found"},
	CodeConflict:           {Status: 409, Title: "Request conflict"},
	CodeResourceLimit:      {Status: 413, Title: "Resource limit exceeded"},
	CodeServiceUnavailable: {Status: 503, Title: "Service unavailable"},
	CodeInternal:           {Status: 500, Title: "Internal error"},
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
		Type:      "https://mindweaver.local/problems/" + strings.ToLower(strings.ReplaceAll(string(code), "_", "-")),
		Title:     spec.Title,
		Status:    spec.Status,
		Detail:    safeDetail,
		Code:      code,
		RequestID: requestID,
	}
}

func (p Problem) Validate() error {
	spec, ok := problemSpecs[p.Code]
	if !ok {
		return fmt.Errorf("unknown problem code %q", p.Code)
	}
	wantType := "https://mindweaver.local/problems/" + strings.ToLower(strings.ReplaceAll(string(p.Code), "_", "-"))
	if p.Status != spec.Status || p.Title != spec.Title || p.Type != wantType {
		return fmt.Errorf("problem fields do not match code %q", p.Code)
	}
	if strings.TrimSpace(p.RequestID) == "" {
		return fmt.Errorf("problem requestId is required")
	}
	return nil
}
