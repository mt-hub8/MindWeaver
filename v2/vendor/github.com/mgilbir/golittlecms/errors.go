package lcms2

import "fmt"

// ErrorCode identifies the class of a library error, mirroring the C
// cmsERROR_* constants.
type ErrorCode uint32

const (
	ErrUndefined          ErrorCode = 0  // cmsERROR_UNDEFINED
	ErrFile               ErrorCode = 1  // cmsERROR_FILE
	ErrRange              ErrorCode = 2  // cmsERROR_RANGE
	ErrInternal           ErrorCode = 3  // cmsERROR_INTERNAL
	ErrNull               ErrorCode = 4  // cmsERROR_NULL
	ErrRead               ErrorCode = 5  // cmsERROR_READ
	ErrSeek               ErrorCode = 6  // cmsERROR_SEEK
	ErrWrite              ErrorCode = 7  // cmsERROR_WRITE
	ErrUnknownExtension   ErrorCode = 8  // cmsERROR_UNKNOWN_EXTENSION
	ErrColorspaceCheck    ErrorCode = 9  // cmsERROR_COLORSPACE_CHECK
	ErrAlreadyDefined     ErrorCode = 10 // cmsERROR_ALREADY_DEFINED
	ErrBadSignature       ErrorCode = 11 // cmsERROR_BAD_SIGNATURE
	ErrCorruptionDetected ErrorCode = 12 // cmsERROR_CORRUPTION_DETECTED
	ErrNotSuitable        ErrorCode = 13 // cmsERROR_NOT_SUITABLE
)

// Error is the error type returned by all fallible operations in this
// package. It carries the lcms2 error class and a human-readable message
// matching the reference implementation's wording where practical.
//
// All exported APIs declare their return type as the plain error interface,
// so this package composes with standard Go error handling; use errors.As to
// recover the *Error and inspect its Code. Internal constructors (errorf,
// signalError) never return a nil *Error, so no typed-nil interface can leak.
type Error struct {
	Code ErrorCode
	Msg  string
}

// Compile-time guarantee that *Error satisfies the error interface.
var _ error = (*Error)(nil)

func (e *Error) Error() string { return e.Msg }

// Is reports whether target matches this error by code, so callers can use
// errors.Is with a sentinel &Error{Code: c}.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Code == e.Code && (t.Msg == "" || t.Msg == e.Msg)
}

// errorf builds an *Error with a formatted message.
func errorf(code ErrorCode, format string, args ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, args...)}
}
