//go:build windows

package ideashook

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

type hookConfigMutationLock struct {
	handle windows.Handle
}

func ensureHookConfigMutationSupported() error { return nil }

func acquireHookConfigMutationLock(ctx context.Context, path string) (io.Closer, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	digest := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(path))))
	name, err := windows.UTF16PtrFromString(`Local\MindWeaver.Ideas.HookConfig.` + hex.EncodeToString(digest[:]))
	if err != nil {
		return nil, err
	}
	runtime.LockOSThread()
	handle, err := windows.CreateMutex(nil, false, name)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		runtime.UnlockOSThread()
		return nil, err
	}
	for {
		if err := contextError(ctx); err != nil {
			_ = windows.CloseHandle(handle)
			runtime.UnlockOSThread()
			return nil, err
		}
		result, waitErr := windows.WaitForSingleObject(handle, 50)
		if waitErr != nil {
			_ = windows.CloseHandle(handle)
			runtime.UnlockOSThread()
			return nil, waitErr
		}
		switch result {
		case windows.WAIT_OBJECT_0, windows.WAIT_ABANDONED:
			return &hookConfigMutationLock{handle: handle}, nil
		case uint32(windows.WAIT_TIMEOUT):
			time.Sleep(time.Millisecond)
		default:
			_ = windows.CloseHandle(handle)
			runtime.UnlockOSThread()
			return nil, errors.New("hook configuration lock wait")
		}
	}
}

func (lock *hookConfigMutationLock) Close() error {
	if lock == nil || lock.handle == 0 {
		return nil
	}
	handle := lock.handle
	lock.handle = 0
	err := errors.Join(windows.ReleaseMutex(handle), windows.CloseHandle(handle))
	runtime.UnlockOSThread()
	return err
}

func writeHookConfigAtomic(ctx context.Context, path string, data []byte) error {
	return writeHookConfigAtomicWithHooks(ctx, path, data, nil, syncHookConfigDirectory)
}

func writeHookConfigAtomicWithHooks(ctx context.Context, path string, data []byte, beforePublish func() error, syncDirectory func(string) error) error {
	parent := filepath.Dir(path)
	if err := ensureHookConfigParent(parent); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(parent, ".mindweaver-hooks-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(temporaryPath)
		}
	}()
	writeErr := writeAll(temporary, data)
	syncErr := temporary.Sync()
	closeErr := temporary.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	if beforePublish != nil {
		if err := beforePublish(); err != nil {
			return err
		}
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	oldPointer, err := windows.UTF16PtrFromString(hookConfigLongPath(temporaryPath))
	if err != nil {
		return err
	}
	newPointer, err := windows.UTF16PtrFromString(hookConfigLongPath(path))
	if err != nil {
		return err
	}
	if err := windows.MoveFileEx(oldPointer, newPointer, windows.MOVEFILE_WRITE_THROUGH); err != nil {
		if errors.Is(err, windows.ERROR_ALREADY_EXISTS) || errors.Is(err, windows.ERROR_FILE_EXISTS) {
			return errors.Join(errHookConfigSnapshotChanged, err)
		}
		return err
	}
	renamed = true
	if err := syncDirectory(parent); err != nil {
		return errors.Join(errHookConfigPublicationUncertain, err)
	}
	return nil
}

func ensureHookConfigParent(path string) error {
	if info, err := os.Lstat(path); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("hook configuration parent is unsafe")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.Join(err, errors.New("hook configuration parent is unsafe"))
	}
	return syncHookConfigDirectory(filepath.Dir(path))
}

func syncHookConfigDirectory(path string) error {
	pointer, err := windows.UTF16PtrFromString(hookConfigLongPath(path))
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(
		pointer,
		windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return err
	}
	return errors.Join(windows.FlushFileBuffers(handle), windows.CloseHandle(handle))
}

func hookConfigLongPath(path string) string {
	if strings.HasPrefix(path, `\\?\`) {
		return path
	}
	if strings.HasPrefix(path, `\\`) {
		return `\\?\UNC\` + strings.TrimPrefix(path, `\\`)
	}
	return `\\?\` + path
}
