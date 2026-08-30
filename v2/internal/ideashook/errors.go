package ideashook

import "errors"

const (
	CodeInvalidInput     = "ideas.hook_invalid_input"
	CodeUnsupported      = "ideas.hook_unsupported"
	CodeStorage          = "ideas.hook_storage_failed"
	CodeIdentityConflict = "ideas.hook_identity_conflict"
	CodeLimitExceeded    = "ideas.hook_limit_exceeded"
	CodeCanceled         = "ideas.hook_canceled"
	CodeIntegrity        = "ideas.hook_integrity_failed"
	CodeNotFound         = "ideas.hook_not_found"
	CodeNoVisibleEvents  = "ideas.hook_no_visible_events"
)

type Error struct {
	code string
	err  error
}

func (err *Error) Error() string { return err.code }
func (err *Error) Unwrap() error { return err.err }
func (err *Error) Code() string  { return err.code }

func hookError(code string, cause error) error { return &Error{code: code, err: cause} }

func CodeOf(err error) string {
	var typed *Error
	if errors.As(err, &typed) {
		return typed.code
	}
	return CodeStorage
}
