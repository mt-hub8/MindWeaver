//go:build windows

package ideashook

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHookConfigMutationLockSerializesAndHonorsContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	first, err := acquireHookConfigMutationLock(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
		defer cancel()
		second, err := acquireHookConfigMutationLock(ctx, path)
		if err == nil {
			_ = second.Close()
		}
		result <- err
	}()
	if err := <-result; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second lock err=%v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	third, err := acquireHookConfigMutationLock(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := third.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestHookConfigAtomicCreateRejectsLateTargetWithoutOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	drifted := []byte(`{"description":"owner","hooks":{}}`)
	err := writeHookConfigAtomicWithHooks(
		context.Background(), path, []byte(`{"description":"mindweaver","hooks":{}}`),
		func() error { return os.WriteFile(path, drifted, 0o600) },
		func(string) error { return nil },
	)
	if !errors.Is(err, errHookConfigSnapshotChanged) || CodeOf(classifyHookConfigWriteError(err)) != CodeIdentityConflict {
		t.Fatalf("err=%v code=%s", err, CodeOf(classifyHookConfigWriteError(err)))
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil || !bytes.Equal(after, drifted) {
		t.Fatalf("drifted config overwritten: %s err=%v", after, readErr)
	}
}

func TestHookConfigDirectorySyncFailureIsPublicationUncertain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	replacement := []byte(`{"description":"mindweaver","hooks":{}}`)
	err := writeHookConfigAtomicWithHooks(context.Background(), path, replacement, nil, func(string) error {
		return errors.New("synthetic directory sync failure")
	})
	if !errors.Is(err, errHookConfigPublicationUncertain) || CodeOf(classifyHookConfigWriteError(err)) != CodePublicationUncertain {
		t.Fatalf("err=%v code=%s", err, CodeOf(classifyHookConfigWriteError(err)))
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil || !bytes.Equal(after, replacement) {
		t.Fatalf("published bytes missing: %s err=%v", after, readErr)
	}
}
