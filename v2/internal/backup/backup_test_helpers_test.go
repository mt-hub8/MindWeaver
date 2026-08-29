//go:build windows

package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mt-hub8/MindWeaver/v2/internal/vault"
)

func readManifest(path string) (Manifest, error) {
	parent, err := openRetainedDirectory(filepath.Dir(path))
	if err != nil {
		return Manifest{}, fmt.Errorf("backup: open manifest parent: %w", err)
	}
	defer parent.Close()
	return readManifestRoot(context.Background(), parent, filepath.Base(path))
}

func verifyBackupTree(ctx context.Context, root string, manifest Manifest) error {
	directory, err := openRetainedDirectory(root)
	if err != nil {
		return err
	}
	defer directory.Close()
	return verifyBackupTreeRoot(ctx, directory, manifest)
}

func resolveArtifactPath(root, relative string) (string, error) {
	if !safeRelativePath(relative) {
		return "", errors.New("backup: artifact path traversal rejected")
	}
	path := filepath.Join(root, filepath.FromSlash(relative))
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("backup: artifact escapes backup root")
	}
	current := root
	components := strings.Split(filepath.FromSlash(relative), string(filepath.Separator))
	for _, component := range components[:len(components)-1] {
		current = filepath.Join(current, component)
		if _, err := vault.ValidateExistingDirectory(current); err != nil {
			return "", fmt.Errorf("backup: artifact parent is unsafe: %w", err)
		}
	}
	return path, nil
}

func removeSQLiteSidecars(path string) error {
	var failures []error
	for _, sidecar := range []string{path + "-wal", path + "-shm"} {
		if err := os.Remove(sidecar); err != nil && !errors.Is(err, os.ErrNotExist) {
			failures = append(failures, fmt.Errorf("remove %s: %w", filepath.Base(sidecar), err))
		}
	}
	return errors.Join(failures...)
}
