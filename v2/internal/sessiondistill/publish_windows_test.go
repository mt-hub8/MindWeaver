//go:build windows

package sessiondistill

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestPublishBundleRejectsNonLocalAndReparseParents(t *testing.T) {
	if err := PublishBundle(context.Background(), `\\server\share\report`, testPublishedBundle(t)); CodeOf(err) != CodeOutputFailed {
		t.Fatalf("UNC publication err=%v code=%s", err, CodeOf(err))
	}

	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		if errors.Is(err, os.ErrPermission) {
			t.Skip("Windows symlink privilege unavailable")
		}
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := PublishBundle(context.Background(), filepath.Join(link, "report"), testPublishedBundle(t)); CodeOf(err) != CodeOutputFailed {
		t.Fatalf("reparse publication err=%v code=%s", err, CodeOf(err))
	}
	if _, err := os.Stat(filepath.Join(target, "report")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reparse publication wrote target: %v", err)
	}
}

func TestPublicationParentHandlePreventsPathReplacement(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	guard, err := retainPublicationParent(parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(guard.witnesses) < 3 {
		_ = guard.Close()
		t.Fatalf("retained namespace witnesses = %d, want root and path components", len(guard.witnesses))
	}
	if err := os.Rename(parent, filepath.Join(root, "replaced")); err == nil {
		_ = guard.Close()
		t.Fatal("retained publication parent allowed replacement")
	}
	if err := guard.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPublicationParentRetainsEveryNamespaceAncestor(t *testing.T) {
	root := t.TempDir()
	ancestor := filepath.Join(root, "ancestor")
	parent := filepath.Join(ancestor, "child", "parent")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	guard, err := retainPublicationParent(parent)
	if err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(root, "moved-ancestor")
	if err := os.Rename(ancestor, moved); !errors.Is(err, windows.ERROR_SHARING_VIOLATION) &&
		!errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		_ = guard.Close()
		t.Fatalf("ancestor rename while retained = %v, want sharing/access denial", err)
	}
	if err := guard.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(ancestor, moved); err != nil {
		t.Fatalf("ancestor rename after close: %v", err)
	}
}

func TestPublicationParentBlocksAncestorJunctionSwapBeforeAbsoluteOperations(t *testing.T) {
	root := t.TempDir()
	ancestor := filepath.Join(root, "race-ancestor")
	parent := filepath.Join(ancestor, "child", "parent")
	canary := filepath.Join(root, "replacement-canary")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(canary, "child", "parent"), 0o700); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(root, "moved-race-ancestor")
	var renameErr error
	var junctionOutput []byte
	guard, err := retainPublicationParentWithHook(parent, func([]*os.File) error {
		renameErr = os.Rename(ancestor, moved)
		if renameErr == nil {
			junctionOutput, _ = exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", ancestor, canary).CombinedOutput()
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	if !errors.Is(renameErr, windows.ERROR_SHARING_VIOLATION) &&
		!errors.Is(renameErr, windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("ancestor swap rename = %v, want sharing/access denial (junction output %q)", renameErr, junctionOutput)
	}
}

func TestPublicationParentRejectsFinalParentSwapBeforeAbsoluteHandle(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(root, "moved-parent")
	guard, err := retainPublicationParentWithHook(parent, func(witnesses []*os.File) error {
		last := len(witnesses) - 1
		if err := witnesses[last].Close(); err != nil {
			return err
		}
		witnesses[last] = nil
		if err := os.Rename(parent, moved); err != nil {
			return err
		}
		return os.Mkdir(parent, 0o700)
	})
	if guard != nil {
		_ = guard.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("final parent replacement err=%v, want identity mismatch", err)
	}
}

func TestPublicationStagingIsRetainedAcrossWriteSyncAndRename(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	guard, err := retainPublicationParent(parent)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	destination := filepath.Join(parent, "ideas")
	bundle := testPublishedBundle(t)
	attempts := 0
	assertStagingRetained := func(staging *retainedPublicationStaging) error {
		attempts++
		moved := filepath.Join(parent, "moved-staging-"+string(rune('0'+attempts)))
		renameErr := os.Rename(staging.path, moved)
		if !errors.Is(renameErr, windows.ERROR_SHARING_VIOLATION) &&
			!errors.Is(renameErr, windows.ERROR_ACCESS_DENIED) {
			return errors.New("retained publication staging allowed rename")
		}
		if _, err := os.Lstat(moved); !errors.Is(err, os.ErrNotExist) {
			return errors.New("retained publication staging exposed a moved identity")
		}
		return nil
	}
	assertChildrenRetained := func(staging *retainedPublicationStaging) error {
		if err := assertStagingRetained(staging); err != nil {
			return err
		}
		moved := filepath.Join(parent, "moved-report.json")
		renameErr := os.Rename(filepath.Join(staging.path, "report.json"), moved)
		if !errors.Is(renameErr, windows.ERROR_SHARING_VIOLATION) &&
			!errors.Is(renameErr, windows.ERROR_ACCESS_DENIED) {
			return errors.New("retained publication file allowed rename")
		}
		return nil
	}
	err = publishBundlePlatformWithHooks(context.Background(), destination, bundle, guard, publicationPlatformHooks{
		afterStagingRetained: assertStagingRetained,
		beforeRename:         assertChildrenRetained,
	})
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("retention attempts=%d, want 2", attempts)
	}
	assertPublishedBundle(t, destination, bundle)
	assertNoStagingDirectories(t, parent)
}

func TestPublicationRejectsUnexpectedStagingEntryBeforeRename(t *testing.T) {
	parent := t.TempDir()
	guard, err := retainPublicationParent(parent)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	destination := filepath.Join(parent, "ideas")
	bundle := testPublishedBundle(t)
	err = publishBundlePlatformWithHooks(context.Background(), destination, bundle, guard, publicationPlatformHooks{
		beforeRename: func(staging *retainedPublicationStaging) error {
			return os.WriteFile(filepath.Join(staging.path, "unverified.txt"), []byte("not part of the bundle"), 0o600)
		},
	})
	if CodeOf(err) != CodeOutputFailed {
		t.Fatalf("unexpected staging entry err=%v code=%s", err, CodeOf(err))
	}
	if _, statErr := os.Lstat(destination); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("unexpected staging entry exposed destination: %v", statErr)
	}
}

func TestPublicationReplacementAfterChildCloseCannotReturnSuccess(t *testing.T) {
	parent := t.TempDir()
	guard, err := retainPublicationParent(parent)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	destination := filepath.Join(parent, "ideas")
	bundle := testPublishedBundle(t)
	replacement := bytes.Repeat([]byte("x"), len(bundle.json))
	err = publishBundlePlatformWithHooks(context.Background(), destination, bundle, guard, publicationPlatformHooks{
		afterChildrenClosed: func(staging *retainedPublicationStaging) error {
			path := filepath.Join(staging.path, "report.json")
			if err := os.Remove(path); err != nil {
				return err
			}
			return os.WriteFile(path, replacement, 0o600)
		},
	})
	if CodeOf(err) != CodePublicationUncertain {
		t.Fatalf("replacement after close err=%v code=%s", err, CodeOf(err))
	}
	got, readErr := os.ReadFile(filepath.Join(destination, "report.json"))
	if readErr != nil || !bytes.Equal(got, replacement) {
		t.Fatalf("replacement witness data=%q err=%v", got, readErr)
	}
}

func TestPublicationUnexpectedEntryAfterChildCloseCannotReturnSuccess(t *testing.T) {
	parent := t.TempDir()
	guard, err := retainPublicationParent(parent)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	destination := filepath.Join(parent, "ideas")
	bundle := testPublishedBundle(t)
	err = publishBundlePlatformWithHooks(context.Background(), destination, bundle, guard, publicationPlatformHooks{
		afterChildrenClosed: func(staging *retainedPublicationStaging) error {
			return os.WriteFile(filepath.Join(staging.path, "unverified.txt"), []byte("not part of the bundle"), 0o600)
		},
	})
	if CodeOf(err) != CodePublicationUncertain {
		t.Fatalf("unexpected entry after close err=%v code=%s", err, CodeOf(err))
	}
	if _, statErr := os.Stat(filepath.Join(destination, "unverified.txt")); statErr != nil {
		t.Fatalf("unexpected entry witness missing: %v", statErr)
	}
}

func TestPublicationStagingRejectsIdentityReplacementBeforeWrite(t *testing.T) {
	root := t.TempDir()
	parentPath := filepath.Join(root, "parent")
	replacementTarget := filepath.Join(root, "replacement-target")
	if err := os.Mkdir(parentPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(replacementTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	guard, err := retainPublicationParent(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	parent, err := guard.parent()
	if err != nil {
		t.Fatal(err)
	}
	name := publicationStagingPrefix + strings.Repeat("a", 32)
	moved := filepath.Join(parentPath, "moved-staging")
	staging, err := createRetainedPublicationStagingNamed(parent, parentPath, name, func(file *os.File, _ os.FileInfo, path string) error {
		if err := file.Close(); err != nil {
			return err
		}
		if err := os.Rename(path, moved); err != nil {
			return err
		}
		output, err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", path, replacementTarget).CombinedOutput()
		if err != nil {
			return errors.Join(err, errors.New(string(output)))
		}
		return nil
	})
	if staging != nil {
		_ = staging.Close()
		t.Fatal("identity-replaced staging was retained")
	}
	if err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("staging replacement err=%v, want identity mismatch", err)
	}
}

func TestRetainedPublicationRenameDoesNotReplaceExistingDestination(t *testing.T) {
	parent := t.TempDir()
	destination := filepath.Join(parent, "ideas")
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	canary := []byte("existing destination")
	if err := os.WriteFile(filepath.Join(destination, "canary.txt"), canary, 0o600); err != nil {
		t.Fatal(err)
	}
	err := PublishBundle(context.Background(), destination, testPublishedBundle(t))
	if CodeOf(err) != CodeDestinationExists {
		t.Fatalf("existing destination err=%v code=%s", err, CodeOf(err))
	}
	got, readErr := os.ReadFile(filepath.Join(destination, "canary.txt"))
	if readErr != nil || string(got) != string(canary) {
		t.Fatalf("existing destination changed: data=%q err=%v", got, readErr)
	}
	assertNoStagingDirectories(t, parent)
}
