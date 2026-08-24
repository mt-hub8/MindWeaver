package blob

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestObjectPinBlocksDeletionEpoch(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	pin, err := store.PinObjectsContext(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	acquired := make(chan *DeletionGuard, 1)
	go func() {
		guard, _ := store.BeginDeletionContext(t.Context())
		acquired <- guard
	}()
	select {
	case guard := <-acquired:
		guard.Release()
		t.Fatal("deletion guard crossed a live object pin")
	case <-time.After(50 * time.Millisecond):
	}
	pin.Release()
	select {
	case guard := <-acquired:
		guard.Release()
	case <-time.After(time.Second):
		t.Fatal("deletion guard did not proceed after pin release")
	}
}

func TestObjectBarrierWaitsAreCancellableWithoutLeakingWriter(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	pin, err := store.PinObjectsContext(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	writerContext, cancelWriter := context.WithTimeout(t.Context(), 25*time.Millisecond)
	defer cancelWriter()
	if guard, err := store.BeginDeletionContext(writerContext); !errors.Is(err, context.DeadlineExceeded) {
		if guard != nil {
			guard.Release()
		}
		t.Fatalf("waiting deletion = %v, %v", guard, err)
	}

	// Cancellation removes the queued writer. A new reader can join the first
	// pin instead of being stranded behind a writer that will acquire later.
	secondContext, cancelSecond := context.WithTimeout(t.Context(), time.Second)
	defer cancelSecond()
	second, err := store.PinObjectsContext(secondContext)
	if err != nil {
		t.Fatalf("pin after canceled writer: %v", err)
	}
	second.Release()
	pin.Release()

	guard, err := store.BeginDeletionContext(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	readerContext, cancelReader := context.WithTimeout(t.Context(), 25*time.Millisecond)
	defer cancelReader()
	if blocked, err := store.PinObjectsContext(readerContext); !errors.Is(err, context.DeadlineExceeded) {
		if blocked != nil {
			blocked.Release()
		}
		t.Fatalf("waiting pin = %v, %v", blocked, err)
	}
	guard.Release()
	if final, err := store.PinObjectsContext(t.Context()); err != nil {
		t.Fatalf("pin after released writer: %v", err)
	} else {
		final.Release()
	}
}

func TestObjectBarrierIsSharedAcrossStoreInstancesForOneRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "blobs")
	first, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	pin, err := second.PinObjectsContext(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	acquired := make(chan *DeletionGuard, 1)
	go func() {
		guard, _ := first.BeginDeletionContext(t.Context())
		acquired <- guard
	}()
	select {
	case guard := <-acquired:
		if guard != nil {
			guard.Release()
		}
		t.Fatal("deletion through first Store bypassed pin on second Store")
	case <-time.After(25 * time.Millisecond):
	}
	pin.Release()
	select {
	case guard := <-acquired:
		if guard == nil {
			t.Fatal("deletion guard was nil")
		}
		guard.Release()
	case <-time.After(time.Second):
		t.Fatal("shared deletion barrier did not resume")
	}
}

func TestDeletionRetrySyncsAlreadyMissingObject(t *testing.T) {
	root := filepath.Join(t.TempDir(), "blobs")
	var failSync atomic.Bool
	sentinel := errors.New("injected directory sync failure")
	store, err := openStore(root, func(path string) error {
		if failSync.Load() && strings.HasSuffix(filepath.ToSlash(path), "/objects/sha256/2c") {
			return sentinel
		}
		return syncDirectory(path)
	}, renamePublished)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := store.Import(t.Context(), strings.NewReader("hello"), 5)
	if err != nil {
		t.Fatal(err)
	}
	guard, err := store.BeginDeletionContext(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Release()
	failSync.Store(true)
	removed, err := guard.Delete(t.Context(), imported.ID)
	if removed || !errors.Is(err, sentinel) {
		t.Fatalf("delete with sync failure = %v, %v", removed, err)
	}
	failSync.Store(false)
	removed, err = guard.Delete(t.Context(), imported.ID)
	if err != nil || removed {
		t.Fatalf("retry missing delete = %v, %v", removed, err)
	}
	digest := strings.TrimPrefix(imported.ID.String(), "sha256:")
	if _, err := os.Lstat(filepath.Join(root, "objects", "sha256", digest[:2], digest[2:])); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted object exists: %v", err)
	}
}
