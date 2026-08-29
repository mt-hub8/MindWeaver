// Package apperror defines transport-neutral errors shared by MindWeaver's
// platform, storage, and domain boundaries.
package apperror

import (
	"context"
	"errors"
	"fmt"
)

// Kind is a stable, coarse-grained error category. Adapters may map a Kind to
// an exit code, HTTP status, or retry policy without coupling the core to a
// transport.
type Kind string

const (
	KindInvalid     Kind = "invalid"
	KindNotFound    Kind = "not_found"
	KindConflict    Kind = "conflict"
	KindUnavailable Kind = "unavailable"
	KindCanceled    Kind = "canceled"
	KindDeadline    Kind = "deadline_exceeded"
	KindInternal    Kind = "internal"
)

// Error carries a stable machine-readable code and a safe public message.
// Err is retained for errors.Is/errors.As and diagnostics, but adapters should
// expose Public rather than Error() to users.
type Error struct {
	Kind     Kind
	Code     string
	Op       string
	Resource string
	Public   string
	Err      error
}

// New creates an application error without an underlying cause.
func New(kind Kind, code, public string) *Error {
	return &Error{Kind: kind, Code: code, Public: public}
}

// Wrap adds application error metadata while preserving the cause. A nil
// cause produces nil, matching the behavior of common wrapping helpers.
func Wrap(err error, kind Kind, code, op, public string) error {
	if err == nil {
		return nil
	}
	return &Error{Kind: kind, Code: code, Op: op, Public: public, Err: err}
}

// WithResource returns a shallow copy annotated with a resource identifier.
// Callers should use a non-secret identifier.
func (e *Error) WithResource(resource string) *Error {
	if e == nil {
		return nil
	}
	copy := *e
	copy.Resource = resource
	return &copy
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	message := e.Code
	if message == "" {
		message = string(e.Kind)
	}
	if e.Op != "" {
		message = e.Op + ": " + message
	}
	if e.Resource != "" {
		message += fmt.Sprintf(" (%s)", e.Resource)
	}
	if e.Err != nil {
		message += ": " + e.Err.Error()
	}
	return message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// KindOf classifies both application errors and context termination.
func KindOf(err error) Kind {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return KindCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return KindDeadline
	}
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Kind
	}
	return KindInternal
}

// CodeOf returns the stable application code, if one is present.
func CodeOf(err error) string {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return ""
}

// PublicMessage returns a safe user-facing message.
func PublicMessage(err error) string {
	if err == nil {
		return ""
	}
	var appErr *Error
	if errors.As(err, &appErr) && appErr.Public != "" {
		return appErr.Public
	}
	switch KindOf(err) {
	case KindCanceled:
		return "operation canceled"
	case KindDeadline:
		return "operation timed out"
	default:
		return "operation failed"
	}
}

// IsKind reports whether err belongs to kind.
func IsKind(err error, kind Kind) bool {
	return KindOf(err) == kind
}
