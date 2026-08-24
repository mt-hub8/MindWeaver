//go:build windows

package backup

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
)

func allowVerifyScratchForTest(*retainedDirectory) error { return nil }

func newVerifyCoordinatorForTest(t *testing.T) *Coordinator {
	t.Helper()
	activePath := filepath.Join(t.TempDir(), "active-vault")
	if err := os.Mkdir(activePath, 0o700); err != nil {
		t.Fatal(err)
	}
	active, err := openRetainedDirectory(activePath)
	if err != nil {
		t.Fatal(err)
	}
	coordinator := &Coordinator{activeVault: active}
	t.Cleanup(func() { _ = coordinator.Close() })
	return coordinator
}

func TestVerifyProvesBackupWithoutWritingSourceAndCleansScratch(t *testing.T) {
	fixture := newBackupFixture(t)
	fixture.addDocument(t, "verify-complete", "read only verification source")
	backupPath := filepath.Join(fixture.root, "verify-source")
	manifest, err := fixture.coordinator.Create(t.Context(), backupPath)
	if err != nil {
		t.Fatal(err)
	}
	scratchParent := filepath.Join(fixture.root, "verify-scratch-parent")
	if err := os.Mkdir(scratchParent, 0o700); err != nil {
		t.Fatal(err)
	}
	saved, err := captureWindowsDACLTree(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := restoreWindowsDACLTree(saved); err != nil {
			t.Errorf("restore source ACLs: %v", err)
		}
	})
	if err := applyCurrentUserReadExecuteDACL(saved); err != nil {
		if errors.Is(err, os.ErrPermission) {
			t.Skipf("cannot apply read-only source ACL on this runner: %v", err)
		}
		t.Fatal(err)
	}

	var progress []Progress
	outcome, err := fixture.coordinator.Verify(t.Context(), backupPath, VerifyOptions{
		ScratchParent: scratchParent,
		Progress: func(snapshot Progress) {
			progress = append(progress, snapshot)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.Succeeded || outcome.Failure != "" {
		t.Fatalf("Verify outcome = %+v", outcome)
	}
	expectedBytes := manifest.Database.Size
	for _, artifact := range manifest.Blobs {
		expectedBytes += artifact.Size
	}
	if outcome.Summary.SchemaVersion != manifest.SchemaVersion ||
		outcome.Summary.ArtifactCount != len(manifest.Blobs)+1 ||
		outcome.Summary.BlobCount != len(manifest.Blobs) ||
		outcome.Summary.VerifiedBytes != expectedBytes {
		t.Fatalf("Verify summary = %+v, manifest=%+v", outcome.Summary, manifest)
	}
	if len(progress) < 7 || progress[0].Phase != PhaseManifest || progress[len(progress)-1].Phase != PhaseComplete {
		t.Fatalf("Verify phases = %+v", progress)
	}
	for index := 1; index < len(progress); index++ {
		if progress[index].ArtifactsVerified < progress[index-1].ArtifactsVerified ||
			progress[index].VerifiedBytes < progress[index-1].VerifiedBytes {
			t.Fatalf("Verify progress regressed at %d: %+v -> %+v", index, progress[index-1], progress[index])
		}
	}
	entries, err := os.ReadDir(scratchParent)
	if err != nil || len(entries) != 0 {
		t.Fatalf("Verify scratch cleanup = %v entries, %v", len(entries), err)
	}
}

func TestVerifyRejectsCanonicalDatabaseCorruptionAfterExactHashes(t *testing.T) {
	fixture := newBackupFixture(t)
	seedBackupAsk(t, fixture.database, "verify-canonical-corrupt")
	backupPath := filepath.Join(fixture.root, "canonical-corrupt-source")
	manifest, err := fixture.coordinator.Create(t.Context(), backupPath)
	if err != nil {
		t.Fatal(err)
	}
	databaseFile := filepath.Join(backupPath, filepath.FromSlash(databasePath))
	mutateBackupSQLite(t, databaseFile, true, `
		UPDATE conversations SET revision = revision + 1 WHERE id = 'verify-canonical-corrupt'
	`)
	root, err := openRetainedDirectory(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Database, err = inspectRootFile(t.Context(), root, databasePath, "")
	closeErr := root.Close()
	if err := errors.Join(err, closeErr); err != nil {
		t.Fatal(err)
	}
	rewriteManifest(t, backupPath, manifest)
	scratchParent := filepath.Join(fixture.root, "canonical-corrupt-scratch")
	if err := os.Mkdir(scratchParent, 0o700); err != nil {
		t.Fatal(err)
	}

	outcome, err := fixture.coordinator.Verify(t.Context(), backupPath, VerifyOptions{ScratchParent: scratchParent})
	if err == nil || outcome.Failure != FailureCorrupt || outcome.Succeeded {
		t.Fatalf("corrupt Verify = %+v, %v", outcome, err)
	}
	if !errors.Is(err, store.ErrCanonicalConsistency) {
		t.Fatalf("corrupt Verify error = %v, want canonical consistency cause", err)
	}
	if err.Error() != "backup operation failed: corrupt" ||
		strings.Contains(err.Error(), backupPath) || strings.Contains(err.Error(), scratchParent) {
		t.Fatalf("Verify exposed unstable/path-bearing error %q", err)
	}
	if entries, err := os.ReadDir(scratchParent); err != nil || len(entries) != 0 {
		t.Fatalf("corrupt Verify scratch cleanup = %v entries, %v", len(entries), err)
	}
}

func TestVerifyDetectsSourceDatabaseChangedAfterQualification(t *testing.T) {
	fixture := newBackupFixture(t)
	fixture.addDocument(t, "verify-source-change", "source changes after scratch qualification")
	backupPath := filepath.Join(fixture.root, "changing-source")
	if _, err := fixture.coordinator.Create(t.Context(), backupPath); err != nil {
		t.Fatal(err)
	}
	scratchParent := filepath.Join(fixture.root, "changing-source-scratch")
	if err := os.Mkdir(scratchParent, 0o700); err != nil {
		t.Fatal(err)
	}
	mutated := false
	_, err := verifyBackup(t.Context(), backupPath, VerifyOptions{ScratchParent: scratchParent}, verifyHooks{
		afterDatabaseQualification: func(source *retainedDirectory, _ *stagingDirectory) error {
			path := filepath.Join(source.path, filepath.FromSlash(databasePath))
			file, err := os.OpenFile(path, os.O_RDWR, 0)
			if err != nil {
				return err
			}
			var original [1]byte
			if _, err := file.ReadAt(original[:], 128); err != nil {
				_ = file.Close()
				return err
			}
			original[0] ^= 0xff
			_, writeErr := file.WriteAt(original[:], 128)
			closeErr := file.Close()
			mutated = writeErr == nil
			return errors.Join(writeErr, closeErr)
		},
	}, allowVerifyScratchForTest)
	if err == nil || !mutated || classifyBackupFailure(err) != FailureCorrupt {
		t.Fatalf("source-change Verify error = %v, mutated=%v", err, mutated)
	}
	if entries, err := os.ReadDir(scratchParent); err != nil || len(entries) != 0 {
		t.Fatalf("source-change scratch cleanup = %v entries, %v", len(entries), err)
	}
}

func TestVerifyRejectsSourceTreeMutationsAfterQualification(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *backupFixture, *retainedDirectory, Manifest) error
	}{
		{
			name: "manifest bytes",
			mutate: func(t *testing.T, _ *backupFixture, source *retainedDirectory, _ Manifest) error {
				path := filepath.Join(source.path, manifestFileName)
				encoded, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				marker := []byte(`"created_at_unix_micros":`)
				offset := bytes.Index(encoded, marker)
				if offset < 0 {
					return errors.New("manifest timestamp field is unavailable")
				}
				offset += len(marker)
				for offset < len(encoded) && (encoded[offset] == ' ' || encoded[offset] == '\t') {
					offset++
				}
				if offset >= len(encoded) || encoded[offset] < '0' || encoded[offset] > '9' {
					return errors.New("manifest timestamp value is unavailable")
				}
				if encoded[offset] == '9' {
					encoded[offset] = '8'
				} else {
					encoded[offset]++
				}
				file, err := os.OpenFile(path, os.O_WRONLY, 0)
				if err != nil {
					return err
				}
				_, writeErr := file.WriteAt(encoded[offset:offset+1], int64(offset))
				return errors.Join(writeErr, file.Close())
			},
		},
		{
			name: "blob bytes",
			mutate: func(t *testing.T, _ *backupFixture, source *retainedDirectory, manifest Manifest) error {
				if len(manifest.Blobs) == 0 {
					return errors.New("test backup has no blob")
				}
				path := filepath.Join(source.path, filepath.FromSlash(manifest.Blobs[0].Path))
				file, err := os.OpenFile(path, os.O_RDWR, 0)
				if err != nil {
					return err
				}
				var value [1]byte
				if _, err := file.ReadAt(value[:], 0); err != nil {
					_ = file.Close()
					return err
				}
				value[0] ^= 0xff
				_, writeErr := file.WriteAt(value[:], 0)
				return errors.Join(writeErr, file.Close())
			},
		},
		{
			name: "extra node",
			mutate: func(t *testing.T, _ *backupFixture, source *retainedDirectory, _ Manifest) error {
				return os.WriteFile(filepath.Join(source.path, "unexpected-node"), []byte("extra"), 0o600)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newBackupFixture(t)
			fixture.addDocument(t, "verify-source-tree-"+strings.ReplaceAll(test.name, " ", "-"), "source mutation evidence")
			backupPath := filepath.Join(fixture.root, "mutation-source")
			manifest, err := fixture.coordinator.Create(t.Context(), backupPath)
			if err != nil {
				t.Fatal(err)
			}
			scratchParent := filepath.Join(fixture.root, "mutation-scratch")
			if err := os.Mkdir(scratchParent, 0o700); err != nil {
				t.Fatal(err)
			}
			mutated := false
			_, err = verifyBackup(t.Context(), backupPath, VerifyOptions{ScratchParent: scratchParent}, verifyHooks{
				afterDatabaseQualification: func(source *retainedDirectory, _ *stagingDirectory) error {
					if err := test.mutate(t, fixture, source, manifest); err != nil {
						return err
					}
					mutated = true
					return nil
				},
			}, allowVerifyScratchForTest)
			if err == nil || !mutated || classifyBackupFailure(err) != FailureCorrupt {
				t.Fatalf("mutated Verify error = %v, mutated=%v", err, mutated)
			}
			if entries, err := os.ReadDir(scratchParent); err != nil || len(entries) != 0 {
				t.Fatalf("mutated Verify scratch cleanup = %v entries, %v", len(entries), err)
			}
		})
	}
}

func TestVerifyRejectsBlobIdentityReplacementWithIdenticalBytes(t *testing.T) {
	fixture := newBackupFixture(t)
	fixture.addDocument(t, "verify-blob-replacement", "same bytes but a different file identity")
	backupPath := filepath.Join(fixture.root, "blob-replacement-source")
	manifest, err := fixture.coordinator.Create(t.Context(), backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Blobs) == 0 {
		t.Fatal("test backup has no blob")
	}
	scratchParent := filepath.Join(fixture.root, "blob-replacement-scratch")
	if err := os.Mkdir(scratchParent, 0o700); err != nil {
		t.Fatal(err)
	}
	replaced := false
	_, err = verifyBackup(t.Context(), backupPath, VerifyOptions{ScratchParent: scratchParent}, verifyHooks{
		afterDatabaseQualification: func(source *retainedDirectory, _ *stagingDirectory) error {
			path := filepath.Join(source.path, filepath.FromSlash(manifest.Blobs[0].Path))
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			moved := filepath.Join(fixture.root, "moved-original-blob")
			if err := os.Rename(path, moved); err != nil {
				return err
			}
			if err := os.WriteFile(path, content, 0o600); err != nil {
				return err
			}
			replaced = true
			return nil
		},
	}, allowVerifyScratchForTest)
	if err == nil || !replaced || classifyBackupFailure(err) != FailureCorrupt {
		t.Fatalf("blob replacement Verify error = %v, replaced=%v", err, replaced)
	}
}

func TestVerifyManifestReplacementIsBlockedOrRejected(t *testing.T) {
	fixture := newBackupFixture(t)
	backupPath := filepath.Join(fixture.root, "manifest-replacement-source")
	if _, err := fixture.coordinator.Create(t.Context(), backupPath); err != nil {
		t.Fatal(err)
	}
	scratchParent := filepath.Join(fixture.root, "manifest-replacement-scratch")
	if err := os.Mkdir(scratchParent, 0o700); err != nil {
		t.Fatal(err)
	}
	replaced := false
	blocked := false
	_, err := verifyBackup(t.Context(), backupPath, VerifyOptions{ScratchParent: scratchParent}, verifyHooks{
		afterDatabaseQualification: func(source *retainedDirectory, _ *stagingDirectory) error {
			path := filepath.Join(source.path, manifestFileName)
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			moved := filepath.Join(fixture.root, "moved-original-manifest")
			if err := os.Rename(path, moved); err != nil {
				blocked = true
				return nil
			}
			if err := os.WriteFile(path, content, 0o600); err != nil {
				return err
			}
			replaced = true
			return nil
		},
	}, allowVerifyScratchForTest)
	if replaced {
		if err == nil || classifyBackupFailure(err) != FailureCorrupt {
			t.Fatalf("manifest replacement Verify error = %v", err)
		}
		return
	}
	if !blocked || err != nil {
		t.Fatalf("manifest replacement = replaced:%v blocked:%v Verify:%v", replaced, blocked, err)
	}
}

func TestVerifyCancellationIsStableAndCleansScratch(t *testing.T) {
	fixture := newBackupFixture(t)
	fixture.addDocument(t, "verify-cancel", "cooperative verify cancellation")
	backupPath := filepath.Join(fixture.root, "cancel-source")
	if _, err := fixture.coordinator.Create(t.Context(), backupPath); err != nil {
		t.Fatal(err)
	}
	scratchParent := filepath.Join(fixture.root, "cancel-scratch")
	if err := os.Mkdir(scratchParent, 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	outcome, err := fixture.coordinator.Verify(ctx, backupPath, VerifyOptions{
		ScratchParent: scratchParent,
		Progress: func(progress Progress) {
			if progress.Phase == PhaseDatabaseCopy {
				cancel()
			}
		},
	})
	if !errors.Is(err, context.Canceled) || outcome.Failure != FailureCanceled || outcome.Succeeded {
		t.Fatalf("canceled Verify = %+v, %v", outcome, err)
	}
	if err.Error() != "backup operation failed: canceled" {
		t.Fatalf("canceled Verify error = %q", err)
	}
	if entries, err := os.ReadDir(scratchParent); err != nil || len(entries) != 0 {
		t.Fatalf("canceled Verify scratch cleanup = %v entries, %v", len(entries), err)
	}
}

func TestVerifyCancellationDuringSourceHashIsNotReportedAsCorruption(t *testing.T) {
	fixture := newBackupFixture(t)
	fixture.addDocument(t, "verify-source-hash-cancel", "cancel before the final source hash pass")
	backupPath := filepath.Join(fixture.root, "source-hash-cancel-source")
	if _, err := fixture.coordinator.Create(t.Context(), backupPath); err != nil {
		t.Fatal(err)
	}
	scratchParent := filepath.Join(fixture.root, "source-hash-cancel-scratch")
	if err := os.Mkdir(scratchParent, 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	outcome, err := fixture.coordinator.Verify(ctx, backupPath, VerifyOptions{
		ScratchParent: scratchParent,
		Progress: func(progress Progress) {
			if progress.Phase == PhaseSourceRecheck {
				cancel()
			}
		},
	})
	if !errors.Is(err, context.Canceled) || outcome.Failure != FailureCanceled || outcome.Succeeded {
		t.Fatalf("source-hash canceled Verify = %+v, %v", outcome, err)
	}
	if entries, err := os.ReadDir(scratchParent); err != nil || len(entries) != 0 {
		t.Fatalf("source-hash canceled scratch cleanup = %v entries, %v", len(entries), err)
	}
}

func TestVerifyCancellationClassificationAtQualificationAndFinalRecheck(t *testing.T) {
	for _, phase := range []Phase{PhaseDatabaseQualification, PhaseCleanup} {
		t.Run(string(phase), func(t *testing.T) {
			fixture := newBackupFixture(t)
			fixture.addDocument(t, "verify-cancel-"+string(phase), "stage-specific cancellation")
			backupPath := filepath.Join(fixture.root, "stage-cancel-source")
			if _, err := fixture.coordinator.Create(t.Context(), backupPath); err != nil {
				t.Fatal(err)
			}
			scratchParent := filepath.Join(fixture.root, "stage-cancel-scratch")
			if err := os.Mkdir(scratchParent, 0o700); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			outcome, err := fixture.coordinator.Verify(ctx, backupPath, VerifyOptions{
				ScratchParent: scratchParent,
				Progress: func(progress Progress) {
					if progress.Phase == phase {
						cancel()
					}
				},
			})
			if !errors.Is(err, context.Canceled) || outcome.Failure != FailureCanceled || outcome.Succeeded {
				t.Fatalf("%s canceled Verify = %+v, %v", phase, outcome, err)
			}
			if entries, err := os.ReadDir(scratchParent); err != nil || len(entries) != 0 {
				t.Fatalf("%s canceled scratch cleanup = %v entries, %v", phase, len(entries), err)
			}
		})
	}
}

func TestVerifyCancellationDuringBlobHashIsStable(t *testing.T) {
	fixture := newBackupFixture(t)
	fixture.addDocument(t, "verify-blob-hash-cancel", "cancel at the blob hash boundary")
	backupPath := filepath.Join(fixture.root, "blob-hash-cancel-source")
	if _, err := fixture.coordinator.Create(t.Context(), backupPath); err != nil {
		t.Fatal(err)
	}
	scratchParent := filepath.Join(fixture.root, "blob-hash-cancel-scratch")
	if err := os.Mkdir(scratchParent, 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	_, err := verifyBackup(ctx, backupPath, VerifyOptions{ScratchParent: scratchParent}, verifyHooks{
		beforeBlobHash: func(int) error {
			cancel()
			return nil
		},
	}, allowVerifyScratchForTest)
	if !errors.Is(err, context.Canceled) || classifyBackupFailure(err) != FailureCanceled {
		t.Fatalf("blob-hash canceled Verify = %v", err)
	}
	if entries, err := os.ReadDir(scratchParent); err != nil || len(entries) != 0 {
		t.Fatalf("blob-hash canceled scratch cleanup = %v entries, %v", len(entries), err)
	}
}

func TestVerifyScratchDACLIsOwnerOnlyAtAtomicCreation(t *testing.T) {
	parentPath := t.TempDir()
	parent, err := openRetainedDirectory(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	checkedAtCreate := false
	staging, err := createStagingDirectoryWithHooks(
		parent,
		verifyScratchPrefix,
		verifyScratchDestination,
		stagingCreationHooks{
			afterAtomicCreate: func(staging *stagingDirectory) error {
				if err := verifyVerifyScratchHandleSecurity(staging.creationWitness, true); err != nil {
					return err
				}
				checkedAtCreate = true
				return nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !checkedAtCreate {
		t.Fatal("verification scratch DACL was not checked at atomic creation")
	}
	if err := cleanupVerifyStaging(staging, parent); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyProgressPanicIsContainedAndScratchIsCleaned(t *testing.T) {
	fixture := newBackupFixture(t)
	backupPath := filepath.Join(fixture.root, "progress-panic-source")
	if _, err := fixture.coordinator.Create(t.Context(), backupPath); err != nil {
		t.Fatal(err)
	}
	scratchParent := filepath.Join(fixture.root, "progress-panic-scratch")
	if err := os.Mkdir(scratchParent, 0o700); err != nil {
		t.Fatal(err)
	}
	outcome, err := fixture.coordinator.Verify(t.Context(), backupPath, VerifyOptions{
		ScratchParent: scratchParent,
		Progress: func(progress Progress) {
			if progress.Phase == PhaseDatabaseCopy {
				panic("callback must not escape")
			}
		},
	})
	if err == nil || outcome.Failure != FailureInternal || outcome.Succeeded {
		t.Fatalf("progress panic Verify = %+v, %v", outcome, err)
	}
	if err.Error() != "backup operation failed: internal" {
		t.Fatalf("progress panic exposed unstable error %q", err)
	}
	if entries, err := os.ReadDir(scratchParent); err != nil || len(entries) != 0 {
		t.Fatalf("progress panic scratch cleanup = %v entries, %v", len(entries), err)
	}
}

func TestVerifyRejectsScratchInsideSourceBeforeWriting(t *testing.T) {
	fixture := newBackupFixture(t)
	backupPath := filepath.Join(fixture.root, "overlap-source")
	if _, err := fixture.coordinator.Create(t.Context(), backupPath); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadDir(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := fixture.coordinator.Verify(t.Context(), backupPath, VerifyOptions{ScratchParent: backupPath})
	if err == nil || outcome.Failure != FailureInvalid {
		t.Fatalf("overlapping Verify = %+v, %v", outcome, err)
	}
	after, err := os.ReadDir(backupPath)
	if err != nil || len(after) != len(before) {
		t.Fatalf("overlap check changed source entries: before=%v after=%v error=%v", before, after, err)
	}
	for index := range before {
		if before[index].Name() != after[index].Name() {
			t.Fatalf("overlap check changed source entry %d: %q -> %q", index, before[index].Name(), after[index].Name())
		}
	}
}

func TestVerifyAndCleanupRejectScratchOverlappingActiveVaultBeforeWriting(t *testing.T) {
	tests := []struct {
		name    string
		scratch func(*testing.T, *backupFixture) string
	}{
		{name: "equal", scratch: func(_ *testing.T, fixture *backupFixture) string { return fixture.vaultRoot }},
		{name: "inside", scratch: func(t *testing.T, fixture *backupFixture) string {
			path := filepath.Join(fixture.vaultRoot, "verification-scratch")
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
			return path
		}},
		{name: "contains", scratch: func(_ *testing.T, fixture *backupFixture) string { return fixture.root }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newBackupFixture(t)
			backupPath := filepath.Join(fixture.root, "active-overlap-source")
			if _, err := fixture.coordinator.Create(t.Context(), backupPath); err != nil {
				t.Fatal(err)
			}
			scratch := test.scratch(t, fixture)
			before, err := os.ReadDir(scratch)
			if err != nil {
				t.Fatal(err)
			}
			outcome, err := fixture.coordinator.Verify(t.Context(), backupPath, VerifyOptions{ScratchParent: scratch})
			if err == nil || outcome.Failure != FailureInvalid || !errors.Is(err, ErrActiveVaultOverlap) {
				t.Fatalf("active-overlap Verify = %+v, %v", outcome, err)
			}
			cleanup, cleanupErr := fixture.coordinator.CleanupVerifyScratch(t.Context(), scratch, 1)
			if cleanupErr == nil || cleanup != (ScratchCleanupSummary{}) ||
				!errors.Is(cleanupErr, ErrActiveVaultOverlap) {
				t.Fatalf("active-overlap cleanup = %+v, %v", cleanup, cleanupErr)
			}
			after, err := os.ReadDir(scratch)
			if err != nil || len(after) != len(before) {
				t.Fatalf("active-overlap changed scratch: before=%d after=%d error=%v", len(before), len(after), err)
			}
			for index := range before {
				if before[index].Name() != after[index].Name() {
					t.Fatalf("active-overlap changed entry %d: %q -> %q", index, before[index].Name(), after[index].Name())
				}
			}
		})
	}
}

func TestVerifyCleanupNeverDeletesReplacementScratch(t *testing.T) {
	fixture := newBackupFixture(t)
	backupPath := filepath.Join(fixture.root, "cleanup-swap-source")
	if _, err := fixture.coordinator.Create(t.Context(), backupPath); err != nil {
		t.Fatal(err)
	}
	scratchParent := filepath.Join(fixture.root, "cleanup-swap-parent")
	if err := os.Mkdir(scratchParent, 0o700); err != nil {
		t.Fatal(err)
	}
	var replacementCanary string
	_, err := verifyBackup(t.Context(), backupPath, VerifyOptions{ScratchParent: scratchParent}, verifyHooks{
		beforeScratchCleanup: func(staging *stagingDirectory) error {
			original := staging.directory.path + "-original"
			if err := errors.Join(
				staging.directory.Close(),
				closeStagingCreationWitness(staging),
				closeResidueLease(staging),
			); err != nil {
				return err
			}
			if err := os.Rename(staging.directory.path, original); err != nil {
				return err
			}
			if err := os.Mkdir(staging.directory.path, 0o700); err != nil {
				return err
			}
			replacementCanary = filepath.Join(staging.directory.path, "replacement-canary")
			return os.WriteFile(replacementCanary, []byte("must survive"), 0o600)
		},
	}, allowVerifyScratchForTest)
	if !errors.Is(err, ErrCleanupResidual) || classifyBackupFailure(err) != FailureCleanupRequired {
		t.Fatalf("replacement cleanup error = %v", err)
	}
	if data, err := os.ReadFile(replacementCanary); err != nil || string(data) != "must survive" {
		t.Fatalf("replacement scratch changed: %q, %v", data, err)
	}
	page, err := ListResidues(t.Context(), scratchParent, 1)
	if err != nil || len(page.Items) != 1 || page.Items[0].State != ResidueStateConflict {
		t.Fatalf("replacement scratch evidence = %+v, %v", page, err)
	}
}

const (
	verifyExitHelperSource  = "MW_BACKUP_VERIFY_SOURCE"
	verifyExitHelperScratch = "MW_BACKUP_VERIFY_SCRATCH"
)

func TestVerifyForcedExitHelper(t *testing.T) {
	source := os.Getenv(verifyExitHelperSource)
	if source == "" {
		return
	}
	_, _ = verifyBackup(
		context.Background(),
		source,
		VerifyOptions{ScratchParent: os.Getenv(verifyExitHelperScratch)},
		verifyHooks{afterDatabaseCopy: func(*stagingDirectory) error {
			os.Exit(97)
			return nil
		}},
		allowVerifyScratchForTest,
	)
	os.Exit(98)
}

func TestCleanupVerifyScratchRecoversBoundForcedExitAndExposesTruncation(t *testing.T) {
	fixture := newBackupFixture(t)
	fixture.addDocument(t, "verify-kill", "verify crash scratch recovery")
	backupPath := filepath.Join(fixture.root, "kill-source")
	if _, err := fixture.coordinator.Create(t.Context(), backupPath); err != nil {
		t.Fatal(err)
	}
	scratchParent := filepath.Join(fixture.root, "kill-scratch")
	if err := os.Mkdir(scratchParent, 0o700); err != nil {
		t.Fatal(err)
	}
	runForcedExit := func() {
		t.Helper()
		command := exec.Command(os.Args[0], "-test.run=^TestVerifyForcedExitHelper$")
		command.Env = append(os.Environ(),
			verifyExitHelperSource+"="+backupPath,
			verifyExitHelperScratch+"="+scratchParent,
		)
		err := command.Run()
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 97 {
			t.Fatalf("Verify helper exit = %v, want 97", err)
		}
	}
	runForcedExit()
	runForcedExit()
	page, err := ListResidues(t.Context(), scratchParent, 10)
	if err != nil || len(page.Items) != 2 || page.Items[0].Kind != "verify" || page.Items[1].Kind != "verify" ||
		page.Items[0].State != ResidueStateStaging {
		t.Fatalf("Verify crash residue = %+v, %v", page, err)
	}
	cleanup, err := fixture.coordinator.CleanupVerifyScratch(t.Context(), scratchParent, 1)
	if err != nil || cleanup.Examined != 1 || cleanup.Removed != 1 || !cleanup.Truncated {
		t.Fatalf("Verify crash cleanup = %+v, %v", cleanup, err)
	}
	cleanup, err = fixture.coordinator.CleanupVerifyScratch(t.Context(), scratchParent, 1)
	if err != nil || cleanup.Examined != 1 || cleanup.Removed != 1 || cleanup.Truncated {
		t.Fatalf("Verify second crash cleanup = %+v, %v", cleanup, err)
	}
	if entries, err := os.ReadDir(scratchParent); err != nil || len(entries) != 0 {
		t.Fatalf("Verify crash scratch remains = %v entries, %v", len(entries), err)
	}
}

func TestCleanupVerifyScratchNeverDeletesAnotherResidueKind(t *testing.T) {
	parentPath := t.TempDir()
	parent, err := openRetainedDirectory(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	staging, err := createStagingDirectory(parent, stagingPrefix, "backup-destination")
	if err != nil {
		_ = parent.Close()
		t.Fatal(err)
	}
	canary := filepath.Join(staging.directory.path, "backup-canary")
	if err := staging.directory.root.WriteFile("backup-canary", []byte("must survive"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := errors.Join(
		staging.directory.Close(),
		closeStagingCreationWitness(staging),
		closeResidueLease(staging),
		parent.Close(),
	); err != nil {
		t.Fatal(err)
	}

	result, err := newVerifyCoordinatorForTest(t).CleanupVerifyScratch(t.Context(), parentPath, 1)
	if err == nil || result.Examined != 1 || result.Removed != 0 {
		t.Fatalf("cross-kind scratch cleanup = %+v, %v", result, err)
	}
	if data, err := os.ReadFile(canary); err != nil || string(data) != "must survive" {
		t.Fatalf("cross-kind scratch cleanup changed backup residue: %q, %v", data, err)
	}
	page, err := ListResidues(t.Context(), parentPath, 1)
	if err != nil || len(page.Items) != 1 || page.Items[0].Kind != "backup" {
		t.Fatalf("cross-kind residue evidence = %+v, %v", page, err)
	}
}
