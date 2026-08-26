//go:build windows

package backup

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsKnownFolderIgnoresLocalAppDataEnvironment(t *testing.T) {
	before, err := windows.KnownFolderPath(
		windows.FOLDERID_LocalAppData,
		windows.KF_FLAG_DONT_VERIFY,
	)
	if err != nil || before == "" {
		t.Fatalf("resolve LocalAppData Known Folder: %q, %v", before, err)
	}
	t.Setenv("LOCALAPPDATA", t.TempDir())
	after, err := windows.KnownFolderPath(
		windows.FOLDERID_LocalAppData,
		windows.KF_FLAG_DONT_VERIFY,
	)
	if err != nil || after != before {
		t.Fatalf("Known Folder changed through LOCALAPPDATA: before=%q after=%q error=%v", before, after, err)
	}
}

func TestPrepareStartupVerifyScratchAtCreatesOwnerOnlyDirectory(t *testing.T) {
	source := t.TempDir()
	trustedParent := t.TempDir()
	scratch, err := prepareStartupVerifyScratchAt(source, trustedParent)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(scratch) != trustedParent || filepath.Base(scratch) != startupVerifyScratchLeaf {
		t.Fatalf("scratch path = %q", scratch)
	}
	retained, err := openRetainedDirectory(scratch)
	if err != nil {
		t.Fatal(err)
	}
	defer retained.Close()
	if err := validateOwnerOnlyVerifyScratch(retained); err != nil {
		t.Fatalf("owner-only scratch validation: %v", err)
	}

	reopened, err := prepareStartupVerifyScratchAt(source, trustedParent)
	if err != nil || reopened != scratch {
		t.Fatalf("reopen secure scratch = %q, %v", reopened, err)
	}
}

func TestPrepareLiveBackupVerifyScratchUsesDistinctOwnerOnlyNamespace(t *testing.T) {
	trustedParent := t.TempDir()
	sourceOne := t.TempDir()
	sourceTwo := t.TempDir()
	leafOne := liveVerifyScratchLeaf(sourceOne)
	leafTwo := liveVerifyScratchLeaf(sourceTwo)
	if leafOne == leafTwo || !hasPlatformReservedPrefix(leafOne, liveVerifyScratchPrefix) ||
		len(leafOne) != len(liveVerifyScratchPrefix)+sha256.Size*2 {
		t.Fatalf("live scratch namespaces = %q / %q", leafOne, leafTwo)
	}
	verificationSource := t.TempDir()
	scratch, err := prepareVerifyScratchAtSources(
		[]string{sourceOne, verificationSource}, trustedParent, leafOne,
	)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(scratch) != trustedParent || filepath.Base(scratch) != leafOne {
		t.Fatalf("live scratch path = %q", scratch)
	}
	retained, err := openRetainedDirectory(scratch)
	if err != nil {
		t.Fatal(err)
	}
	defer retained.Close()
	if err := validateOwnerOnlyVerifyScratch(retained); err != nil {
		t.Fatalf("owner-only live scratch validation: %v", err)
	}
}

func TestPrepareLiveBackupVerifyScratchRejectsVerificationSourceBeforeWrite(t *testing.T) {
	activeVault := t.TempDir()
	verificationSource := t.TempDir()
	trustedParent := filepath.Join(verificationSource, "known-folder")
	if err := os.Mkdir(trustedParent, 0o700); err != nil {
		t.Fatal(err)
	}
	leaf := liveVerifyScratchLeaf(activeVault)
	canary := filepath.Join(verificationSource, "source-canary")
	if err := os.WriteFile(canary, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if scratch, err := prepareVerifyScratchAtSources(
		[]string{activeVault, verificationSource}, trustedParent, leaf,
	); err == nil || scratch != "" {
		t.Fatalf("verification-source overlap = %q, %v", scratch, err)
	}
	if _, err := os.Lstat(filepath.Join(trustedParent, leaf)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("overlap admission wrote scratch: %v", err)
	}
	if content, err := os.ReadFile(canary); err != nil || string(content) != "unchanged" {
		t.Fatalf("overlap admission changed source: %q, %v", content, err)
	}
}

func TestPrepareStartupVerifyScratchRejectsSourceOverlapBeforeWrite(t *testing.T) {
	source := t.TempDir()
	trustedParent := filepath.Join(source, "trusted-parent")
	if err := os.Mkdir(trustedParent, 0o700); err != nil {
		t.Fatal(err)
	}
	canary := filepath.Join(source, "source-canary")
	if err := os.WriteFile(canary, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if scratch, err := prepareStartupVerifyScratchAt(source, trustedParent); err == nil || scratch != "" {
		t.Fatalf("source-containing scratch admission = %q, %v", scratch, err)
	}
	if _, err := os.Lstat(filepath.Join(trustedParent, startupVerifyScratchLeaf)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("overlap admission wrote scratch: %v", err)
	}
	if content, err := os.ReadFile(canary); err != nil || string(content) != "unchanged" {
		t.Fatalf("overlap admission changed source: %q, %v", content, err)
	}

	separateSource := t.TempDir()
	scratch, err := prepareStartupVerifyScratchAt(separateSource, trustedParent)
	if err != nil {
		t.Fatal(err)
	}
	insideCanary := filepath.Join(scratch, "inside-canary")
	if err := os.WriteFile(insideCanary, []byte("still-unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if reopened, err := prepareStartupVerifyScratchAt(scratch, trustedParent); err == nil || reopened != "" {
		t.Fatalf("scratch-as-source admission = %q, %v", reopened, err)
	}
	if content, err := os.ReadFile(insideCanary); err != nil || string(content) != "still-unchanged" {
		t.Fatalf("scratch-as-source admission changed source: %q, %v", content, err)
	}
}

func TestStartupRestoreRecoveryAllowsSiblingSourceButRejectsContainingSource(t *testing.T) {
	parent := t.TempDir()
	source := filepath.Join(parent, "backup-source")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	recovery, err := NewStartupRestoreResidueRecovery(source, parent)
	if err != nil {
		t.Fatalf("sibling source rejected: %v", err)
	}
	if err := recovery.Close(); err != nil {
		t.Fatal(err)
	}

	childParent := filepath.Join(source, "restore-parent")
	if err := os.Mkdir(childParent, 0o700); err != nil {
		t.Fatal(err)
	}
	recovery, err = NewStartupRestoreResidueRecovery(source, childParent)
	if recovery != nil || err == nil || FailureClassOf(err) != FailureInvalid {
		t.Fatalf("source-containing parent recovery = %#v, %v", recovery, err)
	}
}

func TestStartupVerifyCleanupRejectsAnotherVerifyTargetWithoutDeletion(t *testing.T) {
	source := t.TempDir()
	scratch, err := prepareStartupVerifyScratchAt(source, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	parent, err := openRetainedDirectory(scratch)
	if err != nil {
		t.Fatal(err)
	}
	staging, err := createStagingDirectory(parent, verifyScratchPrefix, "another-verify-purpose")
	if err != nil {
		_ = parent.Close()
		t.Fatal(err)
	}
	canary := filepath.Join(staging.directory.path, "canary")
	if err := staging.directory.root.WriteFile("canary", []byte("must-survive"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := errors.Join(
		staging.directory.Close(), closeStagingCreationWitness(staging),
		closeResidueLease(staging), parent.Close(),
	); err != nil {
		t.Fatal(err)
	}

	recovery, err := NewStartupVerifyScratchRecovery(source, scratch)
	if err != nil {
		t.Fatal(err)
	}
	defer recovery.Close()
	summary, err := recovery.Cleanup(t.Context(), 1)
	if err == nil || summary.Removed != 0 || FailureClassOf(err) != FailureInvalid {
		t.Fatalf("foreign verify target cleanup = %+v, %v", summary, err)
	}
	if content, err := os.ReadFile(canary); err != nil || string(content) != "must-survive" {
		t.Fatalf("foreign verify target changed: %q, %v", content, err)
	}
}
