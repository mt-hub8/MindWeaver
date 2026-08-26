//go:build !windows

package backup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func hasPlatformReservedPrefix(name, prefix string) bool { return strings.HasPrefix(name, prefix) }

func validPlatformLeaf(string) bool { return true }

func validPlatformDestinationInput(string) bool { return true }

func validatePlatformDestinationNamespace(string) error { return nil }

func validatePlatformDirectoryNamespace(string) error { return nil }

func validateRetainedDirectoryCaseSemantics(*retainedDirectory) error { return nil }

func retainPlatformDirectoryNamespace(path string) ([]*os.File, os.FileInfo, error) {
	validated, err := existingDirectory(path)
	if err != nil {
		return nil, nil, err
	}
	file, err := os.Open(validated)
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		if err == nil {
			err = errors.New("backup: retained directory witness is not a real directory")
		}
		return nil, nil, errors.Join(err, file.Close())
	}
	return []*os.File{file}, info, nil
}

func retainedManagedDestinationAncestor(directory *retainedDirectory) (bool, error) {
	if directory == nil || directory.root == nil || directory.identity.info == nil {
		return false, errors.New("backup: destination ancestor capability is unavailable")
	}
	return managedDestinationLeaf(filepath.Base(filepath.Clean(directory.path))), nil
}
