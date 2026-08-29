//go:build windows

package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

const ownedStagingCleanupTimeout = 2 * time.Minute

func ensureStagingCleanupSupported() error { return nil }

func ensureResidueRecoverySupported() error { return nil }

// cleanupOwnedStaging deletes only the random staging directory whose identity
// was retained at creation. Every descendant is enumerated in bounded batches
// through os.Root and deleted through a separately verified DELETE handle. A
// swapped leaf, reparse point, mount boundary, unsupported node, count excess,
// or timeout stops cleanup and leaves the remainder for inspection.
func cleanupOwnedStaging(staging *stagingDirectory, parent *retainedDirectory, kind string) error {
	if staging == nil || staging.directory == nil || parent == nil || parent.root == nil {
		return errors.Join(ErrCleanupResidual, errors.New("backup: invalid staging cleanup input"))
	}
	expected := staging.directory.identity
	if _, err := verifyLeafIdentity(parent, staging.name, expected); err != nil {
		return errors.Join(
			ErrCleanupResidual,
			ErrCleanupIdentityLost,
			staging.directory.Close(),
			fmt.Errorf("backup: retain %s staging after identity loss: %w", kind, err),
		)
	}
	if staging.directory.root == nil {
		reopened, err := openExpectedDirectory(parent, staging.name, expected)
		if err != nil {
			return errors.Join(
				ErrCleanupResidual,
				ErrCleanupIdentityLost,
				fmt.Errorf("backup: reopen retained staging identity for cleanup: %w", err),
			)
		}
		staging.directory = reopened
	}
	ctx, cancel := context.WithTimeout(context.Background(), ownedStagingCleanupTimeout)
	defer cancel()
	if err := removeOwnedStagingContents(ctx, staging.directory); err != nil {
		closeErr := staging.directory.Close()
		return errors.Join(
			ErrCleanupResidual,
			fmt.Errorf("backup: bounded %s staging cleanup stopped at %s: %w", kind, staging.directory.path, err),
			closeErr,
		)
	}
	if err := closeStagingCreationWitness(staging); err != nil {
		return errors.Join(
			ErrCleanupResidual,
			fmt.Errorf("backup: close %s staging witness before explicit recovery: %w", kind, err),
			staging.directory.Close(),
		)
	}
	deleteRoot, err := prepareOwnedDeletion(staging.directory, ".", expected.info)
	if err != nil {
		return errors.Join(
			ErrCleanupResidual,
			fmt.Errorf("backup: retain %s staging root after delete-handle failure: %w", kind, err),
			staging.directory.Close(),
		)
	}
	closeRootErr := staging.directory.Close()
	deleteErr := markOwnedDeletion(deleteRoot)
	closeDeleteErr := deleteRoot.Close()
	if err := errors.Join(deleteErr, closeDeleteErr); err != nil {
		return errors.Join(
			ErrCleanupResidual,
			closeRootErr,
			fmt.Errorf("backup: retain %s staging root after identity-bound delete failure: %w", kind, err),
		)
	}
	_, statErr := parent.root.Lstat(staging.name)
	if statErr == nil {
		return errors.Join(
			ErrCleanupResidual,
			ErrCleanupIdentityLost,
			closeRootErr,
			errors.New("backup: a staging leaf remains after deleting the retained identity"),
		)
	}
	if !errors.Is(statErr, os.ErrNotExist) {
		return errors.Join(ErrCleanupResidual, closeRootErr, statErr)
	}
	if err := parent.acquireSyncHandle(); err != nil {
		return errors.Join(
			ErrCleanupResidual,
			closeRootErr,
			fmt.Errorf("backup: acquire staging parent sync after cleanup: %w", err),
		)
	}
	if err := errors.Join(closeRootErr, syncRetainedDirectory(parent)); err != nil {
		return errors.Join(ErrCleanupResidual, err)
	}
	return nil
}

func removeOwnedStagingContents(ctx context.Context, root *retainedDirectory) (resultErr error) {
	rootInfo, err := root.root.Stat(".")
	if err != nil {
		return err
	}
	rootCursor, err := openExactTreeDirectory(root, rootInfo, ".")
	if err != nil {
		return err
	}
	stack := []*exactTreeDirectory{rootCursor}
	defer func() {
		for _, directory := range stack {
			if directory.file != nil {
				resultErr = errors.Join(resultErr, directory.file.Close())
			}
		}
	}()
	visited := 1
	for len(stack) != 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		current := stack[len(stack)-1]
		entry, hasEntry, readErr := nextExactTreeEntry(current)
		if readErr != nil {
			return readErr
		}
		if !hasEntry {
			stack = stack[:len(stack)-1]
			if current.relative == "." {
				if err := closeExactTreeDirectory(root, rootInfo, current); err != nil {
					return err
				}
				continue
			}
			deleteHandle, err := prepareOwnedDeletion(root, current.relative, current.identity)
			if err != nil {
				return err
			}
			closeCursorErr := closeExactTreeDirectory(root, rootInfo, current)
			deleteErr := markOwnedDeletion(deleteHandle)
			closeDeleteErr := deleteHandle.Close()
			if err := errors.Join(closeCursorErr, deleteErr, closeDeleteErr); err != nil {
				return err
			}
			if _, err := root.root.Lstat(filepath.FromSlash(current.relative)); !errors.Is(err, os.ErrNotExist) {
				if err == nil {
					err = errors.New("backup: deleted staging directory identity still has a path")
				}
				return err
			}
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		visited++
		if visited > maxExactTreeEntries {
			return fmt.Errorf("backup: staging cleanup exceeds entry limit %d", maxExactTreeEntries)
		}
		relative := filepath.ToSlash(filepath.Join(filepath.FromSlash(current.relative), entry.Name()))
		if current.relative == "." {
			relative = filepath.ToSlash(entry.Name())
		}
		info, err := root.root.Lstat(filepath.FromSlash(relative))
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 || info.Mode()&os.ModeSymlink != 0 {
			return errors.Join(ErrFilesystemBoundary, fmt.Errorf("backup: link appeared in staging cleanup: %s", relative))
		}
		if info.IsDir() {
			if current.depth >= maxCleanupDepth {
				return fmt.Errorf("backup: staging cleanup exceeds directory depth limit %d", maxCleanupDepth)
			}
			child, err := openExactTreeDirectory(root, rootInfo, relative)
			if err != nil {
				return err
			}
			stack = append(stack, child)
			continue
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("backup: unsupported staging cleanup node: %s", relative)
		}
		if err := deleteOwnedEntry(root, relative, info); err != nil {
			return err
		}
	}
	return nil
}

func deleteOwnedEntry(root *retainedDirectory, relative string, expected os.FileInfo) error {
	handle, err := prepareOwnedDeletion(root, relative, expected)
	if err != nil {
		return err
	}
	deleteErr := markOwnedDeletion(handle)
	closeErr := handle.Close()
	if err := errors.Join(deleteErr, closeErr); err != nil {
		return err
	}
	if _, err := root.root.Lstat(filepath.FromSlash(relative)); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			err = errors.New("backup: deleted staging entry identity still has a path")
		}
		return err
	}
	return nil
}

func prepareOwnedDeletion(root *retainedDirectory, relative string, expected os.FileInfo) (*os.File, error) {
	if root == nil || root.root == nil || expected == nil {
		return nil, errors.New("backup: invalid identity-bound deletion")
	}
	path := root.path
	if relative != "." {
		path = filepath.Join(root.path, filepath.FromSlash(relative))
	}
	pointer, err := windows.UTF16PtrFromString(longWindowsPath(path))
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		pointer,
		windows.DELETE|windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, err
	}
	fail := func(cause error) (*os.File, error) {
		_ = windows.CloseHandle(handle)
		return nil, cause
	}
	var information windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &information); err != nil {
		return fail(err)
	}
	if information.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fail(errors.Join(ErrFilesystemBoundary, errors.New("backup: reparse entry rejected during cleanup")))
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		return fail(errors.New("backup: adopt staging delete handle"))
	}
	handleInfo, statErr := file.Stat()
	current, currentErr := root.root.Lstat(filepath.FromSlash(relative))
	if err := errors.Join(statErr, currentErr); err != nil ||
		current.Mode()&os.ModeSymlink != 0 ||
		current.IsDir() != expected.IsDir() || handleInfo.IsDir() != expected.IsDir() ||
		!os.SameFile(expected, current) || !os.SameFile(expected, handleInfo) {
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		return nil, errors.Join(ErrCleanupIdentityLost, errors.New("backup: staging entry changed before deletion"))
	}
	return file, nil
}

func markOwnedDeletion(file *os.File) error {
	deleteFile := byte(1)
	return windows.SetFileInformationByHandle(
		windows.Handle(file.Fd()), windows.FileDispositionInfo, &deleteFile, 1,
	)
}

func removeOwnedReceiptFile(parent *retainedDirectory, name string, expected os.FileInfo) error {
	file, err := prepareOwnedDeletion(parent, name, expected)
	if err != nil {
		return err
	}
	deleteErr := markOwnedDeletion(file)
	closeErr := file.Close()
	if err := errors.Join(deleteErr, closeErr); err != nil {
		return err
	}
	if _, err := parent.root.Lstat(name); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			err = errors.New("backup: residue receipt remains after identity-bound deletion")
		}
		return err
	}
	return nil
}
