package platform_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/mt-hub8/MindWeaver/v2/platform"
	"github.com/mt-hub8/MindWeaver/v2/platform/apperror"
)

func TestRandomIDGeneratorProducesCanonicalVersion4ID(t *testing.T) {
	generator := platform.RandomIDGenerator{Reader: bytes.NewReader(make([]byte, 16))}
	id, err := generator.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := id.String(), "00000000-0000-4000-8000-000000000000"; got != want {
		t.Fatalf("New() = %q, want %q", got, want)
	}
	parsed, err := platform.ParseID(id.String())
	if err != nil || parsed != id {
		t.Fatalf("ParseID() = %q, %v", parsed, err)
	}
}

func TestIDGeneratorHonorsCanceledContextBeforeEntropyRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	generator := platform.RandomIDGenerator{Reader: bytes.NewReader(nil)}
	_, err := generator.New(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("New() error = %v, want context.Canceled", err)
	}
}

func TestParseIDRejectsMalformedInput(t *testing.T) {
	_, err := platform.ParseID("not-an-id")
	if !apperror.IsKind(err, apperror.KindInvalid) || apperror.CodeOf(err) != "id.invalid" {
		t.Fatalf("ParseID() error = %v", err)
	}
}
