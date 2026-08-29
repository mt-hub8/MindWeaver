package apperror_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/mt-hub8/MindWeaver/v2/platform/apperror"
)

func TestWrapPreservesCauseAndMetadata(t *testing.T) {
	cause := errors.New("disk details must stay internal")
	err := apperror.Wrap(cause, apperror.KindUnavailable, "storage.unavailable", "config.load", "configuration is unavailable")
	err = fmt.Errorf("startup: %w", err)

	if !errors.Is(err, cause) {
		t.Fatal("wrapped cause is not discoverable")
	}
	if got := apperror.KindOf(err); got != apperror.KindUnavailable {
		t.Fatalf("KindOf() = %q, want %q", got, apperror.KindUnavailable)
	}
	if got := apperror.CodeOf(err); got != "storage.unavailable" {
		t.Fatalf("CodeOf() = %q", got)
	}
	if got := apperror.PublicMessage(err); got != "configuration is unavailable" {
		t.Fatalf("PublicMessage() = %q", got)
	}
}

func TestContextClassificationTakesPrecedence(t *testing.T) {
	err := apperror.Wrap(context.Canceled, apperror.KindInternal, "internal", "run", "failed")
	if got := apperror.KindOf(err); got != apperror.KindCanceled {
		t.Fatalf("KindOf() = %q, want canceled", got)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatal("context cancellation is not preserved")
	}

	err = fmt.Errorf("outer: %w", context.DeadlineExceeded)
	if got := apperror.KindOf(err); got != apperror.KindDeadline {
		t.Fatalf("KindOf() = %q, want deadline", got)
	}
}

func TestUnknownErrorsAreSafeByDefault(t *testing.T) {
	err := errors.New("password=do-not-leak")
	if got := apperror.KindOf(err); got != apperror.KindInternal {
		t.Fatalf("KindOf() = %q, want internal", got)
	}
	if got := apperror.PublicMessage(err); got != "operation failed" {
		t.Fatalf("PublicMessage() = %q", got)
	}
}
