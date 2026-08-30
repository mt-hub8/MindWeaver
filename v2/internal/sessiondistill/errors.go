package sessiondistill

import (
	"errors"
	"fmt"
)

const (
	CodeInvalidArgument      = "sessiondistill.invalid_argument"
	CodeUnsupportedSchema    = "sessiondistill.unsupported_schema"
	CodeInputLimitExceeded   = "sessiondistill.input_limit_exceeded"
	CodeOutputLimitExceeded  = "sessiondistill.output_limit_exceeded"
	CodeCanceled             = "sessiondistill.canceled"
	CodeOutputFailed         = "sessiondistill.output_failed"
	CodePublicationUncertain = "sessiondistill.publication_uncertain"
	CodeDestinationExists    = "sessiondistill.destination_exists"
	CodeUnsupportedPlatform  = "sessiondistill.unsupported_platform"
)

type Error struct {
	code string
	err  error
}

func (err *Error) Error() string { return err.code }
func (err *Error) Unwrap() error { return err.err }
func (err *Error) Code() string  { return err.code }

func newError(code string, cause error) error {
	return &Error{code: code, err: cause}
}

func CodeOf(err error) string {
	var typed *Error
	if errors.As(err, &typed) {
		return typed.code
	}
	return "sessiondistill.internal"
}

func invalid() error {
	return newError(CodeInvalidArgument, fmt.Errorf("invalid distillation input"))
}
