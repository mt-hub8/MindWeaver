//go:build windows

package ideashook

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const ownerFileAllAccess windows.ACCESS_MASK = 0x001f01ff

var pendingFilePattern = regexp.MustCompile(`^\.pending-[0-9]{5}-[0-9a-f]{64}-[0-9a-f]{32}$`)

type spoolLock struct {
	file *os.File
}

func resolveSpoolRoot() (string, error) {
	localAppData, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, windows.KF_FLAG_DONT_VERIFY)
	if err != nil || strings.TrimSpace(localAppData) == "" {
		return "", errors.New("local application data is unavailable")
	}
	return filepath.Join(localAppData, "MindWeaver-Ideas-Codex-Spool-v1"), nil
}

func prepareSpoolRoot(root string) error {
	abs, err := filepath.Abs(root)
	if err != nil || filepath.Clean(abs) != filepath.Clean(root) {
		return errors.New("spool root is not canonical")
	}
	if err := validateFixedLocalPath(abs); err != nil {
		return err
	}
	if err := validateExistingPathComponents(filepath.Dir(abs)); err != nil {
		return err
	}
	return ensureOwnerOnlyDirectory(abs)
}

func validateFixedLocalPath(path string) error {
	if strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, `\\?\`) || strings.HasPrefix(path, `\\.\`) {
		return errors.New("spool root must be local")
	}
	volume := filepath.VolumeName(path)
	if len(volume) != 2 || volume[1] != ':' {
		return errors.New("spool root must use a drive letter")
	}
	root := strings.ToUpper(volume[:1]) + `:\`
	pointer, err := windows.UTF16PtrFromString(root)
	if err != nil || windows.GetDriveType(pointer) != windows.DRIVE_FIXED {
		return errors.New("spool root must use fixed local media")
	}
	buffer := make([]uint16, windows.MAX_PATH+1)
	if err := windows.GetVolumeNameForVolumeMountPoint(pointer, &buffer[0], uint32(len(buffer))); err != nil {
		return errors.New("spool root drive is not registered")
	}
	return nil
}

func validateExistingPathComponents(path string) error {
	clean := filepath.Clean(path)
	volume := filepath.VolumeName(clean)
	current := volume + string(os.PathSeparator)
	remainder := strings.TrimPrefix(clean, current)
	for _, component := range strings.Split(remainder, string(os.PathSeparator)) {
		if component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("spool parent is not a real directory")
		}
		pointer, pointerErr := windows.UTF16PtrFromString(longWindowsPath(current))
		if pointerErr != nil {
			return pointerErr
		}
		attributes, err := windows.GetFileAttributes(pointer)
		if err != nil || attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return errors.New("spool parent is a reparse point")
		}
	}
	return nil
}

func ensureOwnerOnlyDirectory(path string) error {
	_, err := ensureOwnerOnlyDirectoryCreated(path)
	return err
}

func ensureOwnerOnlyDirectoryCreated(path string) (bool, error) {
	created := false
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		descriptor, descriptorErr := ownerOnlyDescriptor(true)
		if descriptorErr != nil {
			return false, descriptorErr
		}
		pointer, pointerErr := windows.UTF16PtrFromString(longWindowsPath(path))
		if pointerErr != nil {
			return false, pointerErr
		}
		attributes := windows.SecurityAttributes{
			Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
			SecurityDescriptor: descriptor,
		}
		if err := windows.CreateDirectory(pointer, &attributes); err != nil {
			if !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
				return false, err
			}
		} else {
			created = true
		}
		info, err = os.Lstat(path)
	}
	if err != nil || info == nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return created, errors.Join(err, errors.New("spool directory is unsafe"))
	}
	file, err := openOwnerOnlyDirectory(path)
	if err != nil {
		return created, err
	}
	verifyErr := verifyOwnerOnlyHandle(file, true)
	closeErr := file.Close()
	if err := errors.Join(verifyErr, closeErr); err != nil {
		return created, err
	}
	if created {
		return true, syncOwnerDirectory(filepath.Dir(path))
	}
	return false, nil
}

func createOwnerOnlyFile(path string) (*os.File, error) {
	attributes, err := ownerSecurityAttributes(false)
	if err != nil {
		return nil, err
	}
	return openFileHandle(path, windows.CREATE_NEW, attributes, windows.GENERIC_READ|windows.GENERIC_WRITE|windows.READ_CONTROL)
}

// createOwnerOnlyStagingFile retains DELETE access while deliberately denying
// delete and write sharing. The same handle is therefore the only capability
// that can change or rename the staged record before publication.
func createOwnerOnlyStagingFile(path string) (*os.File, error) {
	attributes, err := ownerSecurityAttributes(false)
	if err != nil {
		return nil, err
	}
	pointer, err := windows.UTF16PtrFromString(longWindowsPath(path))
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		pointer,
		windows.GENERIC_READ|windows.GENERIC_WRITE|windows.READ_CONTROL|windows.DELETE,
		windows.FILE_SHARE_READ,
		attributes,
		windows.CREATE_NEW,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("adopt spool staging handle")
	}
	if err := verifyOwnerOnlyHandle(file, false); err != nil {
		return nil, errors.Join(err, markSpoolHandleForDeletion(file), file.Close())
	}
	return file, nil
}

func openOwnerOnlyFile(path string) (*os.File, error) {
	file, err := openFileHandle(path, windows.OPEN_EXISTING, nil, windows.GENERIC_READ|windows.READ_CONTROL)
	if err != nil {
		return nil, err
	}
	if err := verifyOwnerOnlyHandle(file, false); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func openOwnerOnlyFileForDeletion(path string) (*os.File, error) {
	file, err := openFileHandle(path, windows.OPEN_EXISTING, nil, windows.GENERIC_READ|windows.READ_CONTROL|windows.DELETE)
	if err != nil {
		return nil, err
	}
	if err := verifyOwnerOnlyHandle(file, false); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func openOrCreateOwnerOnlyFile(path string) (*os.File, error) {
	attributes, err := ownerSecurityAttributes(false)
	if err != nil {
		return nil, err
	}
	file, err := openFileHandle(path, windows.CREATE_NEW, attributes, windows.GENERIC_READ|windows.GENERIC_WRITE|windows.READ_CONTROL)
	if err == nil {
		if verifyErr := verifyOwnerOnlyHandle(file, false); verifyErr != nil {
			_ = file.Close()
			return nil, verifyErr
		}
		return file, nil
	}
	if !errors.Is(err, windows.ERROR_FILE_EXISTS) && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return nil, err
	}
	file, err = openFileHandle(path, windows.OPEN_EXISTING, nil, windows.GENERIC_READ|windows.GENERIC_WRITE|windows.READ_CONTROL)
	if err != nil {
		return nil, err
	}
	if err := verifyOwnerOnlyHandle(file, false); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func openFileHandle(path string, disposition uint32, attributes *windows.SecurityAttributes, access uint32) (*os.File, error) {
	pointer, err := windows.UTF16PtrFromString(longWindowsPath(path))
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		pointer,
		access,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		attributes,
		disposition,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("adopt spool file handle")
	}
	return file, nil
}

func openDirectoryHandle(path string) (*os.File, error) {
	pointer, err := windows.UTF16PtrFromString(longWindowsPath(path))
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		pointer,
		windows.GENERIC_READ|windows.GENERIC_WRITE|windows.READ_CONTROL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("adopt spool directory handle")
	}
	return file, nil
}

func openOwnerOnlyDirectory(path string) (*os.File, error) {
	pointer, err := windows.UTF16PtrFromString(longWindowsPath(path))
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		pointer,
		windows.GENERIC_READ|windows.READ_CONTROL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("adopt spool directory handle")
	}
	if err := verifyOwnerOnlyHandle(file, true); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func openOwnerOnlyDirectoryForDeletion(path string) (*os.File, error) {
	pointer, err := windows.UTF16PtrFromString(longWindowsPath(path))
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		pointer,
		windows.GENERIC_READ|windows.READ_CONTROL|windows.DELETE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("adopt spool directory deletion handle")
	}
	if err := verifyOwnerOnlyHandle(file, true); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func secureOwnerOnlyFile(file *os.File) error {
	return verifyOwnerOnlyHandle(file, false)
}

func ownerSecurityAttributes(inherit bool) (*windows.SecurityAttributes, error) {
	descriptor, err := ownerOnlyDescriptor(inherit)
	if err != nil {
		return nil, err
	}
	return &windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: descriptor,
	}, nil
}

func ownerOnlyDescriptor(inherit bool) (*windows.SECURITY_DESCRIPTOR, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil {
		return nil, errors.New("resolve spool owner")
	}
	flags := ""
	if inherit {
		flags = "OICI"
	}
	descriptor, err := windows.SecurityDescriptorFromString(
		"O:" + user.User.Sid.String() + "D:P(A;" + flags + ";FA;;;" + user.User.Sid.String() + ")",
	)
	if err != nil {
		return nil, errors.New("construct spool security descriptor")
	}
	return descriptor, nil
}

func verifyOwnerOnlyHandle(file *os.File, directory bool) error {
	if file == nil {
		return errors.New("spool handle is unavailable")
	}
	handle := windows.Handle(file.Fd())
	var information windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &information); err != nil {
		return err
	}
	if information.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		(directory && information.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0) ||
		(!directory && (information.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 || information.NumberOfLinks != 1)) {
		return errors.New("spool object type is unsafe")
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil {
		return errors.New("resolve spool owner")
	}
	security, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := security.Owner()
	if err != nil || owner == nil || !owner.Equals(user.User.Sid) {
		return errors.New("spool owner mismatch")
	}
	control, _, err := security.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return errors.New("spool access list is not protected")
	}
	dacl, _, err := security.DACL()
	if err != nil || dacl == nil || dacl.AceCount != 1 {
		return errors.New("spool access list is not owner-only")
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil || ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Mask != ownerFileAllAccess {
		return errors.New("spool access entry is invalid")
	}
	wantFlags := uint8(0)
	if directory {
		wantFlags = windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE
	}
	if ace.Header.AceFlags != wantFlags || !(*windows.SID)(unsafe.Pointer(&ace.SidStart)).Equals(user.User.Sid) {
		return errors.New("spool access entry is not owner-only")
	}
	return nil
}

func acquireSpoolLock(ctx context.Context, root string) (*spoolLock, error) {
	file, err := openOrCreateOwnerOnlyFile(filepath.Join(root, ".capture.lock"))
	if err != nil {
		return nil, hookError(CodeStorage, err)
	}
	for {
		var overlapped windows.Overlapped
		err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped)
		if err == nil {
			return &spoolLock{file: file}, nil
		}
		if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			_ = file.Close()
			return nil, hookError(CodeStorage, err)
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			_ = file.Close()
			return nil, hookError(CodeCanceled, ctx.Err())
		case <-timer.C:
		}
	}
}

func (lock *spoolLock) Close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	var overlapped windows.Overlapped
	unlockErr := windows.UnlockFileEx(windows.Handle(lock.file.Fd()), 0, 1, 0, &overlapped)
	closeErr := lock.file.Close()
	lock.file = nil
	return errors.Join(unlockErr, closeErr)
}

func cleanupPendingRecords(directory string, entries []os.DirEntry) ([]os.DirEntry, error) {
	retained := make([]os.DirEntry, 0, len(entries))
	pending := make([]os.DirEntry, 0, min(len(entries), maxPendingRecords))
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".pending-") {
			retained = append(retained, entry)
			continue
		}
		if !pendingFilePattern.MatchString(entry.Name()) || entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil, errors.New("invalid pending spool record")
		}
		pending = append(pending, entry)
		if len(pending) > maxPendingRecords {
			return nil, errors.New("pending spool record limit")
		}
	}
	for _, entry := range pending {
		path := filepath.Join(directory, entry.Name())
		file, err := openOwnerOnlyFileForDeletion(path)
		if err != nil {
			return nil, err
		}
		deleteErr := markSpoolHandleForDeletion(file)
		closeErr := file.Close()
		if err := errors.Join(deleteErr, closeErr); err != nil {
			return nil, err
		}
	}
	if len(pending) != 0 {
		if err := syncOwnerDirectory(directory); err != nil {
			return nil, err
		}
	}
	return retained, nil
}

func removeEmptyOwnerOnlyDirectory(path string) error {
	directory, entries, err := openBoundedOwnerOnlyDirectory(path, 0)
	if err != nil {
		return err
	}
	expected, statErr := directory.Stat()
	if len(entries) != 0 {
		return errors.Join(statErr, directory.Close(), errors.New("spool directory is not empty"))
	}
	closeErr := directory.Close()
	if err := errors.Join(statErr, closeErr); err != nil {
		return err
	}
	deletionHandle, err := openOwnerOnlyDirectoryForDeletion(path)
	if err != nil {
		return err
	}
	current, currentErr := deletionHandle.Stat()
	remaining, readErr := readBoundedDirectoryEntries(deletionHandle, 0)
	if currentErr != nil || readErr != nil || expected == nil || current == nil || !os.SameFile(expected, current) || len(remaining) != 0 {
		return errors.Join(currentErr, readErr, deletionHandle.Close(), errors.New("spool directory identity changed before cleanup"))
	}
	deleteErr := markSpoolHandleForDeletion(deletionHandle)
	closeErr = deletionHandle.Close()
	if err := errors.Join(readErr, deleteErr, closeErr); err != nil {
		return err
	}
	return syncOwnerDirectory(filepath.Dir(path))
}

func writeOwnerOnlyRecord(ctx context.Context, directoryHandle *os.File, directory, fileName string, content []byte, hooks captureSpoolHooks) (resultErr error) {
	if !eventFilePattern.MatchString(fileName) || len(content) == 0 || len(content) > maxSpoolRecordBytes {
		return errors.New("invalid spool record")
	}
	if err := verifyOwnerOnlyHandle(directoryHandle, true); err != nil {
		return err
	}
	randomSuffix := make([]byte, 16)
	if _, err := rand.Read(randomSuffix); err != nil {
		return err
	}
	base := strings.TrimSuffix(fileName, ".json")
	temporaryName := ".pending-" + base + "-" + hex.EncodeToString(randomSuffix)
	temporaryPath := filepath.Join(directory, temporaryName)
	file, err := createOwnerOnlyStagingFile(temporaryPath)
	if err != nil {
		return err
	}
	published := false
	defer func() {
		if !published {
			resultErr = errors.Join(resultErr, markSpoolHandleForDeletion(file))
		}
		resultErr = errors.Join(resultErr, file.Close())
	}()
	writeErr := writeAll(file, content)
	verifyErr := secureOwnerOnlyFile(file)
	syncErr := file.Sync()
	stat, statErr := file.Stat()
	if statErr == nil && (stat == nil || !stat.Mode().IsRegular() || stat.Size() != int64(len(content))) {
		statErr = errors.New("spool staging identity changed")
	}
	if err := errors.Join(writeErr, verifyErr, syncErr, statErr); err != nil {
		return err
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	if hooks.beforeRecordRename != nil {
		if err := hooks.beforeRecordRename(temporaryPath); err != nil {
			return err
		}
	}
	if err := secureOwnerOnlyFile(file); err != nil {
		return err
	}
	if err := renameRetainedSpoolRecord(file, directoryHandle, fileName); err != nil {
		return err
	}
	published = true
	if hooks.beforeDirectorySync != nil {
		if err := hooks.beforeDirectorySync(); err != nil {
			return err
		}
	}
	return syncOwnerDirectory(directory)
}

func renameRetainedSpoolRecord(file, directory *os.File, targetName string) error {
	if file == nil || directory == nil || !eventFilePattern.MatchString(targetName) {
		return errors.New("invalid retained spool rename")
	}
	name, err := windows.UTF16FromString(targetName)
	if err != nil {
		return err
	}
	name = name[:len(name)-1]
	type fileRenameInformation struct {
		ReplaceIfExists byte
		_               [7]byte
		RootDirectory   windows.Handle
		FileNameLength  uint32
		FileName        [1]uint16
	}
	header := fileRenameInformation{}
	nameOffset := unsafe.Offsetof(header.FileName)
	buffer := make([]byte, int(nameOffset)+len(name)*2)
	information := (*fileRenameInformation)(unsafe.Pointer(&buffer[0]))
	information.RootDirectory = windows.Handle(directory.Fd())
	information.FileNameLength = uint32(len(name) * 2)
	copy(unsafe.Slice(&information.FileName[0], len(name)), name)
	err = windows.NtSetInformationFile(
		windows.Handle(file.Fd()),
		&windows.IO_STATUS_BLOCK{},
		(*byte)(unsafe.Pointer(information)),
		uint32(len(buffer)),
		windows.FileRenameInformation,
	)
	if status, ok := err.(windows.NTStatus); ok {
		if status == windows.STATUS_OBJECT_NAME_COLLISION || status == windows.STATUS_OBJECT_NAME_EXISTS {
			return os.ErrExist
		}
		return status.Errno()
	}
	return err
}

func markSpoolHandleForDeletion(file *os.File) error {
	if file == nil {
		return nil
	}
	deleteFile := byte(1)
	return windows.SetFileInformationByHandle(
		windows.Handle(file.Fd()), windows.FileDispositionInfo, &deleteFile, 1,
	)
}

func syncOwnerDirectory(path string) error {
	file, err := openDirectoryHandle(path)
	if err != nil {
		return err
	}
	flushErr := windows.FlushFileBuffers(windows.Handle(file.Fd()))
	closeErr := file.Close()
	return errors.Join(flushErr, closeErr)
}

func longWindowsPath(path string) string {
	if strings.HasPrefix(path, `\\?\`) {
		return path
	}
	if strings.HasPrefix(path, `\\`) {
		return `\\?\UNC\` + strings.TrimPrefix(path, `\\`)
	}
	return `\\?\` + path
}
