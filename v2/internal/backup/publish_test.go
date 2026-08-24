//go:build windows

package backup

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func acceptPublishedContent(ctx context.Context, _ *retainedDirectory) error {
	return ctx.Err()
}

func committedSingleFile(relative string, content []byte) publicationVerifier {
	digest := fmt.Sprintf("%x", sha256.Sum256(content))
	artifact := Artifact{Path: relative, Size: int64(len(content)), SHA256: digest}
	return func(ctx context.Context, root *retainedDirectory) error {
		if err := verifyRootArtifact(ctx, root, artifact); err != nil {
			return err
		}
		entries, err := fs.ReadDir(root.root.FS(), ".")
		if err != nil {
			return err
		}
		if len(entries) != 1 || entries[0].Name() != relative || !entries[0].Type().IsRegular() {
			return errors.New("published tree differs from single-file commitment")
		}
		return nil
	}
}

func TestPublishDirectoryNeverReplacesRacingDestination(t *testing.T) {
	parent := t.TempDir()
	staging := filepath.Join(parent, "staging")
	destination := filepath.Join(parent, "destination")
	if err := os.Mkdir(staging, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "new"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	// This models another launcher creating the destination after the initial
	// newDestination check but before publication.
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "owner"), []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	retainedParent, err := openRetainedDirectory(parent)
	if err != nil {
		t.Fatal(err)
	}
	defer retainedParent.Close()
	if err := retainedParent.acquireSyncHandle(); err != nil {
		t.Fatal(err)
	}
	stagingInfo, err := os.Lstat(staging)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publishDirectory(
		staging,
		destination,
		retainedParent,
		filepath.Base(destination),
		publicationAttempt{
			sourceName: filepath.Base(staging),
			expected:   directoryIdentity{info: stagingInfo},
		},
	); err == nil {
		t.Fatal("publication unexpectedly replaced a racing destination")
	}
	data, err := os.ReadFile(filepath.Join(destination, "owner"))
	if err != nil || string(data) != "existing" {
		t.Fatalf("racing destination changed: %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(staging, "new")); err != nil {
		t.Fatalf("failed publication consumed staging tree: %v", err)
	}
}

func TestPublishPreparedDirectoryRejectsLeafSwapAfterStagingClose(t *testing.T) {
	parentPath := t.TempDir()
	parent, err := openRetainedDirectory(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	staging, err := createStagingDirectory(parent, stagingPrefix, "destination")
	if err != nil {
		t.Fatal(err)
	}
	defer closeResidueLease(staging)
	if err := staging.directory.root.WriteFile("owned", []byte("original staging"), 0o600); err != nil {
		t.Fatal(err)
	}
	destinationPath := filepath.Join(parentPath, "destination")
	target := &destinationTarget{
		finalPath: destinationPath,
		finalName: filepath.Base(destinationPath),
		parent:    parent,
	}
	movedStaging := staging.directory.path + "-original"
	canaryPath := filepath.Join(staging.directory.path, "replacement-canary")

	publication, err := publishPreparedDirectory(t.Context(), staging, target, acceptPublishedContent, publicationHooks{
		afterStagingCloseBeforeRename: func(string) error {
			if err := os.Rename(staging.directory.path, movedStaging); err != nil {
				return err
			}
			if err := os.Mkdir(staging.directory.path, 0o700); err != nil {
				return err
			}
			return os.WriteFile(canaryPath, []byte("replacement tree"), 0o600)
		},
	})
	if !errors.Is(err, ErrPublicationUncertain) {
		t.Fatalf("leaf swap error = %v, want ErrPublicationUncertain", err)
	}
	if publication.stagingConsumed {
		t.Fatal("pre-rename identity failure incorrectly consumed staging")
	}
	if _, err := os.Lstat(destinationPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("leaf swap unexpectedly published a destination: %v", err)
	}
	if data, err := os.ReadFile(canaryPath); err != nil || string(data) != "replacement tree" {
		t.Fatalf("replacement canary changed: %q, %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(movedStaging, "owned")); err != nil || string(data) != "original staging" {
		t.Fatalf("original staging changed: %q, %v", data, err)
	}
}

func TestPublishPreparedDirectoryRejectsDescendantOverwriteAfterVerification(t *testing.T) {
	parentPath := t.TempDir()
	parent, err := openRetainedDirectory(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	staging, err := createStagingDirectory(parent, stagingPrefix, "destination")
	if err != nil {
		t.Fatal(err)
	}
	defer closeResidueLease(staging)
	original := []byte("committed publication content")
	if err := staging.directory.root.WriteFile("owned", original, 0o600); err != nil {
		t.Fatal(err)
	}
	destinationPath := filepath.Join(parentPath, "destination")
	outsideCanary := filepath.Join(parentPath, "outside-canary")
	if err := os.WriteFile(outsideCanary, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}

	publication, err := publishPreparedDirectory(
		t.Context(),
		staging,
		&destinationTarget{
			finalPath: destinationPath,
			finalName: filepath.Base(destinationPath),
			parent:    parent,
		},
		committedSingleFile("owned", original),
		publicationHooks{
			afterStagingCloseBeforeRename: func(stagingPath string) error {
				return os.WriteFile(filepath.Join(stagingPath, "owned"), []byte("tampered"), 0o600)
			},
		},
	)
	if !errors.Is(err, ErrPublicationUncertain) {
		t.Fatalf("descendant overwrite error = %v, want ErrPublicationUncertain", err)
	}
	if !publication.stagingConsumed {
		t.Fatal("post-rename verification failure did not report consumed staging")
	}
	if data, err := os.ReadFile(filepath.Join(destinationPath, "owned")); err != nil || string(data) != "tampered" {
		t.Fatalf("uncertain target was deleted or changed: %q, %v", data, err)
	}
	if data, err := os.ReadFile(outsideCanary); err != nil || string(data) != "outside" {
		t.Fatalf("outside canary was deleted or changed: %q, %v", data, err)
	}
}

func TestPublicationRejectsContentMutationBySuccessfulParentSync(t *testing.T) {
	parentPath := t.TempDir()
	parent, err := openRetainedDirectory(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	staging, err := createStagingDirectory(parent, stagingPrefix, "destination")
	if err != nil {
		t.Fatal(err)
	}
	defer closeResidueLease(staging)
	original := []byte("content committed before publication")
	if err := staging.directory.root.WriteFile("owned", original, 0o600); err != nil {
		t.Fatal(err)
	}
	destinationPath := filepath.Join(parentPath, "destination")
	outsideCanary := filepath.Join(parentPath, "outside-canary")
	if err := os.WriteFile(outsideCanary, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = publishPreparedDirectory(
		t.Context(),
		staging,
		&destinationTarget{
			finalPath: destinationPath,
			finalName: filepath.Base(destinationPath),
			parent:    parent,
		},
		committedSingleFile("owned", original),
		publicationHooks{
			syncParent: func(*retainedDirectory) error {
				return os.WriteFile(filepath.Join(destinationPath, "owned"), []byte("tampered during sync"), 0o600)
			},
		},
	)
	if !errors.Is(err, ErrPublicationUncertain) {
		t.Fatalf("successful sync mutation error = %v, want ErrPublicationUncertain", err)
	}
	if data, err := os.ReadFile(filepath.Join(destinationPath, "owned")); err != nil || string(data) != "tampered during sync" {
		t.Fatalf("uncertain destination was deleted or changed: %q, %v", data, err)
	}
	if data, err := os.ReadFile(outsideCanary); err != nil || string(data) != "outside" {
		t.Fatalf("outside canary was deleted or changed: %q, %v", data, err)
	}
}

func TestPublicationCancellationAfterRenameIsUncertainAndPreserved(t *testing.T) {
	parentPath := t.TempDir()
	parent, err := openRetainedDirectory(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	staging, err := createStagingDirectory(parent, stagingPrefix, "destination")
	if err != nil {
		t.Fatal(err)
	}
	defer closeResidueLease(staging)
	if err := staging.directory.root.WriteFile("owned", []byte("published"), 0o600); err != nil {
		t.Fatal(err)
	}
	destinationPath := filepath.Join(parentPath, "destination")
	ctx, cancel := context.WithCancel(t.Context())
	_, err = publishPreparedDirectory(
		ctx,
		staging,
		&destinationTarget{
			finalPath: destinationPath,
			finalName: filepath.Base(destinationPath),
			parent:    parent,
		},
		func(ctx context.Context, _ *retainedDirectory) error {
			cancel()
			return ctx.Err()
		},
		publicationHooks{},
	)
	if !errors.Is(err, ErrPublicationUncertain) || !errors.Is(err, context.Canceled) {
		t.Fatalf("post-rename cancellation error = %v, want canceled ErrPublicationUncertain", err)
	}
	if data, err := os.ReadFile(filepath.Join(destinationPath, "owned")); err != nil || string(data) != "published" {
		t.Fatalf("canceled publication target was deleted or changed: %q, %v", data, err)
	}
}

func TestPublicationSyncFailureNeverDeletesSwappedDestination(t *testing.T) {
	parentPath := t.TempDir()
	parent, err := openRetainedDirectory(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	staging, err := createStagingDirectory(parent, stagingPrefix, "destination")
	if err != nil {
		t.Fatal(err)
	}
	defer closeResidueLease(staging)
	if err := staging.directory.root.WriteFile("owned", []byte("published staging"), 0o600); err != nil {
		t.Fatal(err)
	}
	destinationPath := filepath.Join(parentPath, "destination")
	target := &destinationTarget{
		finalPath: destinationPath,
		finalName: filepath.Base(destinationPath),
		parent:    parent,
	}
	movedDestination := destinationPath + "-original"
	canaryPath := filepath.Join(destinationPath, "replacement-canary")
	syncFailure := errors.New("injected parent sync failure")

	_, err = publishPreparedDirectory(t.Context(), staging, target, acceptPublishedContent, publicationHooks{
		syncParent: func(*retainedDirectory) error { return syncFailure },
		afterSyncFailure: func() error {
			if err := os.Rename(destinationPath, movedDestination); err != nil {
				return err
			}
			if err := os.Mkdir(destinationPath, 0o700); err != nil {
				return err
			}
			return os.WriteFile(canaryPath, []byte("replacement tree"), 0o600)
		},
	})
	if !errors.Is(err, ErrPublicationUncertain) || !errors.Is(err, syncFailure) {
		t.Fatalf("sync failure error = %v, want injected failure and ErrPublicationUncertain", err)
	}
	if data, err := os.ReadFile(canaryPath); err != nil || string(data) != "replacement tree" {
		t.Fatalf("replacement destination was deleted or changed: %q, %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(movedDestination, "owned")); err != nil || string(data) != "published staging" {
		t.Fatalf("original published tree changed: %q, %v", data, err)
	}
}

func TestPublicationSyncFailurePreservesVerifiedDestination(t *testing.T) {
	parentPath := t.TempDir()
	parent, err := openRetainedDirectory(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	staging, err := createStagingDirectory(parent, stagingPrefix, "destination")
	if err != nil {
		t.Fatal(err)
	}
	defer closeResidueLease(staging)
	if err := staging.directory.root.WriteFile("owned", []byte("published staging"), 0o600); err != nil {
		t.Fatal(err)
	}
	destinationPath := filepath.Join(parentPath, "destination")
	syncFailure := errors.New("injected parent sync failure")
	_, err = publishPreparedDirectory(t.Context(), staging, &destinationTarget{
		finalPath: destinationPath,
		finalName: filepath.Base(destinationPath),
		parent:    parent,
	}, acceptPublishedContent, publicationHooks{
		syncParent: func(*retainedDirectory) error { return syncFailure },
	})
	if !errors.Is(err, ErrPublicationUncertain) || !errors.Is(err, syncFailure) {
		t.Fatalf("sync failure error = %v, want injected failure and ErrPublicationUncertain", err)
	}
	if data, err := os.ReadFile(filepath.Join(destinationPath, "owned")); err != nil || string(data) != "published staging" {
		t.Fatalf("uncertain published tree changed: %q, %v", data, err)
	}
}

func TestCleanupStagingTreeNeverDeletesReplacementLeaf(t *testing.T) {
	parentPath := t.TempDir()
	parent, err := openRetainedDirectory(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	staging, err := createStagingDirectory(parent, stagingPrefix, "destination")
	if err != nil {
		t.Fatal(err)
	}
	defer closeResidueLease(staging)
	if err := staging.directory.root.WriteFile("owned", []byte("original staging"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := staging.directory.Close(); err != nil {
		t.Fatal(err)
	}
	if err := closeStagingCreationWitness(staging); err != nil {
		t.Fatal(err)
	}
	movedStaging := staging.directory.path + "-original"
	if err := os.Rename(staging.directory.path, movedStaging); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(staging.directory.path, 0o700); err != nil {
		t.Fatal(err)
	}
	canaryPath := filepath.Join(staging.directory.path, "replacement-canary")
	if err := os.WriteFile(canaryPath, []byte("replacement tree"), 0o600); err != nil {
		t.Fatal(err)
	}

	err = cleanupStagingTree(staging, parent, "attack-test")
	if !errors.Is(err, ErrCleanupIdentityLost) || !errors.Is(err, ErrCleanupResidual) {
		t.Fatalf("cleanup error = %v, want identity-lost residual", err)
	}
	if data, err := os.ReadFile(canaryPath); err != nil || string(data) != "replacement tree" {
		t.Fatalf("replacement staging was deleted or changed: %q, %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(movedStaging, "owned")); err != nil || string(data) != "original staging" {
		t.Fatalf("original staging changed: %q, %v", data, err)
	}
}

func TestCleanupStagingTreeHandlesVerifiedEmptyLeafByPlatformPolicy(t *testing.T) {
	parentPath := t.TempDir()
	parent, err := openRetainedDirectory(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	staging, err := createStagingDirectory(parent, stagingPrefix, "destination")
	if err != nil {
		t.Fatal(err)
	}
	defer closeResidueLease(staging)
	err = cleanupStagingTree(staging, parent, "empty-test")
	if err != nil {
		t.Fatalf("Windows Tier-1 cleanup error = %v", err)
	}
	if _, err := os.Lstat(staging.directory.path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("verified empty staging remains after cleanup: %v", err)
	}
}
