//go:build windows

package vault

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	ioctlStorageGetHotplugInfo = 0x002d0c14

	hresultInvalidFunction       = 0x80070001
	hresultCloudNotUnderSyncRoot = 0x80070186
	hresultNotACloudSyncRoot     = 0x80070195
	cfSyncRootInfoBasic          = 0
)

var cfGetSyncRootInfoByHandle = windows.NewLazySystemDLL("cldapi.dll").NewProc("CfGetSyncRootInfoByHandle")

type windowsFileIDInfo struct {
	VolumeSerialNumber uint64
	FileID             [16]byte
}

type windowsFileIdentity struct {
	VolumeSerialNumber uint32
	FileIDInfo         windowsFileIDInfo
}

type storageHotplugInfo struct {
	Size                     uint32
	MediaRemovable           byte
	MediaHotplug             byte
	DeviceHotplug            byte
	WriteCacheEnableOverride byte
}

func validateRealDirectory(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("%w: resolve directory: %v", ErrUnsafePath, err)
	}
	abs = filepath.Clean(abs)

	volume := filepath.VolumeName(abs)
	if volume == "" {
		return fmt.Errorf("%w: directory has no volume", ErrUnsafePath)
	}
	current := volume + string(os.PathSeparator)
	remainder := strings.TrimPrefix(abs, current)
	for _, component := range strings.Split(remainder, string(os.PathSeparator)) {
		if component == "" {
			continue
		}
		current = filepath.Join(current, component)
		handle, err := openPathHandle(current, true, true, true)
		if err != nil {
			return fmt.Errorf("%w: open %q without following reparse points: %v", ErrUnsafePath, component, err)
		}
		verifyErr := verifyHandleType(handle, true)
		closeErr := windows.CloseHandle(handle)
		if err := errors.Join(verifyErr, closeErr); err != nil {
			return fmt.Errorf("%w: validate %q: %v", ErrUnsafePath, component, err)
		}
	}
	return nil
}

func validateActiveVaultLocation(path string) error {
	if err := validateVaultPathSpelling(path); err != nil {
		return err
	}
	probe := path
	if _, err := os.Lstat(probe); errors.Is(err, os.ErrNotExist) {
		probe = filepath.Dir(path)
	} else if err != nil {
		return fmt.Errorf("inspect Vault location: %w", err)
	}
	if err := validateRealDirectory(probe); err != nil {
		return err
	}
	handle, err := openPathHandle(probe, true, true, true)
	if err != nil {
		return fmt.Errorf("%w: open Vault location: %v", ErrUnsafePath, err)
	}
	defer windows.CloseHandle(handle)
	if err := verifyHandleType(handle, true); err != nil {
		return err
	}
	return validateWindowsVaultHandle(handle, probe)
}

func openVaultRootHandle(path string) (*os.File, string, error) {
	if err := validateRealDirectory(path); err != nil {
		return nil, "", err
	}
	handle, err := openPathHandle(path, true, true, false)
	if err != nil {
		return nil, "", fmt.Errorf("open retained Vault root handle: %w", err)
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = windows.CloseHandle(handle)
		}
	}()
	if err := verifyHandleType(handle, true); err != nil {
		return nil, "", err
	}
	if err := validateWindowsVaultHandle(handle, path); err != nil {
		return nil, "", err
	}
	displayPath, err := finalHandlePath(handle)
	if err != nil {
		return nil, "", fmt.Errorf("%w: resolve retained Vault root: %v", ErrUnsafePath, err)
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		return nil, "", errors.New("adopt retained Vault root handle")
	}
	closeOnError = false
	return file, normalizeFinalPath(displayPath), nil
}

func verifyRootIdentity(retained, probe *os.File) error {
	retainedIdentity, err := fileIdentity(windows.Handle(retained.Fd()))
	if err != nil {
		return fmt.Errorf("inspect retained root identity: %w", err)
	}
	probeIdentity, err := fileIdentity(windows.Handle(probe.Fd()))
	if err != nil {
		return fmt.Errorf("inspect os.Root identity: %w", err)
	}
	if retainedIdentity != probeIdentity {
		return fmt.Errorf("%w: retained root handles identify different directories", ErrUnsafePath)
	}
	return nil
}

func validateControlledDirectory(rootFile *os.File, name string) error {
	handle, err := openRelativeHandle(rootFile, name, windows.FILE_GENERIC_READ,
		windows.FILE_OPEN, windows.FILE_ATTRIBUTE_NORMAL,
		windows.FILE_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_REPARSE_POINT,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE)
	if err != nil {
		return fmt.Errorf("%w: open controlled directory %q without reparsing: %v", ErrUnsafePath, name, err)
	}
	verifyErr := verifyHandleType(handle, true)
	closeErr := windows.CloseHandle(handle)
	if err := errors.Join(verifyErr, closeErr); err != nil {
		return fmt.Errorf("%w: validate controlled directory %q: %v", ErrUnsafePath, name, err)
	}
	return nil
}

func acquireProcessLock(rootFile *os.File) (*os.File, error) {
	handle, err := openRelativeHandle(rootFile, lockFileName,
		windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE,
		windows.FILE_OPEN_IF, windows.FILE_ATTRIBUTE_HIDDEN,
		windows.FILE_NON_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_REPARSE_POINT,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE)
	if err != nil {
		return nil, fmt.Errorf("%w: open Vault lock leaf: %v", ErrUnsafePath, err)
	}
	file := os.NewFile(uintptr(handle), lockFileName)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("adopt Vault lock handle")
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = file.Close()
		}
	}()

	var information windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &information); err != nil {
		return nil, fmt.Errorf("inspect Vault lock handle: %w", err)
	}
	if information.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		information.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 || information.NumberOfLinks != 1 {
		return nil, fmt.Errorf("%w: lock is not a private regular non-reparse file", ErrUnsafePath)
	}
	var overlapped windows.Overlapped
	err = windows.LockFileEx(handle,
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, &overlapped)
	if err != nil {
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("acquire Vault byte-range lock: %w", err)
	}
	closeOnError = false
	return file, nil
}

func releaseProcessLock(file *os.File) error {
	var overlapped windows.Overlapped
	unlockErr := windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlapped)
	closeErr := file.Close()
	return errors.Join(unlockErr, closeErr)
}

func syncRetainedDirectory(*os.File) error { return nil }

func openRelativeHandle(rootFile *os.File, name string, access, disposition, attributes, options, share uint32) (windows.Handle, error) {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return windows.InvalidHandle, err
	}
	objectAttributes := windows.OBJECT_ATTRIBUTES{
		Length:        uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		RootDirectory: windows.Handle(rootFile.Fd()),
		ObjectName:    objectName,
		Attributes:    windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	var status windows.IO_STATUS_BLOCK
	var handle windows.Handle
	err = windows.NtCreateFile(&handle, access, &objectAttributes, &status, nil,
		attributes, share, disposition, options, 0, 0)
	return handle, err
}

func openPathHandle(path string, directory, openReparsePoint, shareDelete bool) (windows.Handle, error) {
	pointer, err := windows.UTF16PtrFromString(longPath(path))
	if err != nil {
		return windows.InvalidHandle, err
	}
	flags := uint32(0)
	if directory {
		flags |= windows.FILE_FLAG_BACKUP_SEMANTICS
	}
	if openReparsePoint {
		flags |= windows.FILE_FLAG_OPEN_REPARSE_POINT
	}
	share := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE)
	if shareDelete {
		share |= windows.FILE_SHARE_DELETE
	}
	return windows.CreateFile(pointer, windows.FILE_READ_ATTRIBUTES, share, nil,
		windows.OPEN_EXISTING, flags, 0)
}

func verifyHandleType(handle windows.Handle, wantDirectory bool) error {
	var information windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &information); err != nil {
		return fmt.Errorf("%w: inspect opened handle: %v", ErrUnsafePath, err)
	}
	if information.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("%w: opened handle is a reparse point", ErrUnsafePath)
	}
	isDirectory := information.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0
	if isDirectory != wantDirectory {
		return fmt.Errorf("%w: opened handle has unexpected type", ErrUnsafePath)
	}
	return nil
}

// verifyHandlePath is retained for low-level tests and diagnostics. Identity is
// volume/file-ID based; the expected path is opened only to obtain its handle.
func verifyHandlePath(handle windows.Handle, expected string, wantDirectory bool) error {
	if err := verifyHandleType(handle, wantDirectory); err != nil {
		return err
	}
	expectedHandle, err := openPathHandle(expected, wantDirectory, true, true)
	if err != nil {
		return fmt.Errorf("%w: open expected identity: %v", ErrUnsafePath, err)
	}
	defer windows.CloseHandle(expectedHandle)
	if err := verifyHandleType(expectedHandle, wantDirectory); err != nil {
		return err
	}
	actualIdentity, err := fileIdentity(handle)
	if err != nil {
		return fmt.Errorf("%w: inspect opened identity: %v", ErrUnsafePath, err)
	}
	expectedIdentity, err := fileIdentity(expectedHandle)
	if err != nil {
		return fmt.Errorf("%w: inspect expected identity: %v", ErrUnsafePath, err)
	}
	if actualIdentity != expectedIdentity {
		return fmt.Errorf("%w: opened handle identifies a different file", ErrUnsafePath)
	}
	return nil
}

func fileIdentity(handle windows.Handle) (windowsFileIdentity, error) {
	var idInfo windowsFileIDInfo
	if err := windows.GetFileInformationByHandleEx(handle, windows.FileIdInfo,
		(*byte)(unsafe.Pointer(&idInfo)), uint32(unsafe.Sizeof(idInfo))); err != nil {
		return windowsFileIdentity{}, err
	}
	var volumeName [windows.MAX_PATH + 1]uint16
	var fileSystemName [32]uint16
	var volumeSerial, maximumComponentLength, fileSystemFlags uint32
	if err := windows.GetVolumeInformationByHandle(handle,
		&volumeName[0], uint32(len(volumeName)), &volumeSerial,
		&maximumComponentLength, &fileSystemFlags,
		&fileSystemName[0], uint32(len(fileSystemName))); err != nil {
		return windowsFileIdentity{}, err
	}
	return windowsFileIdentity{VolumeSerialNumber: volumeSerial, FileIDInfo: idInfo}, nil
}

func validateWindowsVaultHandle(handle windows.Handle, path string) error {
	if err := verifyHandleType(handle, true); err != nil {
		return err
	}
	volumePath, err := volumePathName(path)
	if err != nil {
		return fmt.Errorf("%w: resolve Vault volume: %v", ErrUnsafeMedia, err)
	}
	driveType, err := driveTypeForVolume(volumePath)
	if err != nil {
		return fmt.Errorf("%w: classify Vault drive: %v", ErrUnsafeMedia, err)
	}
	if err := classifyDriveType(driveType); err != nil {
		return err
	}
	if err := rejectMappedDrive(path); err != nil {
		return err
	}

	var volumeLabel [windows.MAX_PATH + 1]uint16
	var fileSystemName [32]uint16
	var volumeSerial, maximumComponentLength, fileSystemFlags uint32
	if err := windows.GetVolumeInformationByHandle(handle,
		&volumeLabel[0], uint32(len(volumeLabel)), &volumeSerial,
		&maximumComponentLength, &fileSystemFlags,
		&fileSystemName[0], uint32(len(fileSystemName))); err != nil {
		return fmt.Errorf("%w: inspect Vault filesystem: %v", ErrUnsafeMedia, err)
	}
	if err := classifyFileSystem(windows.UTF16ToString(fileSystemName[:])); err != nil {
		return err
	}
	volumeName, err := volumeNameForMountPoint(volumePath)
	if err != nil {
		return fmt.Errorf("%w: resolve Vault volume identity: %v", ErrUnsafeMedia, err)
	}
	hotplug, err := queryHotplugInfo(volumeName)
	if err != nil {
		return fmt.Errorf("%w: classify Vault storage device: %v", ErrUnsafeMedia, err)
	}
	if err := classifyHotplugInfo(hotplug); err != nil {
		return err
	}
	if err := rejectCloudSyncRoot(handle); err != nil {
		return err
	}
	if _, err := fileIdentity(handle); err != nil {
		return fmt.Errorf("%w: obtain stable Vault identity: %v", ErrUnsafePath, err)
	}
	return nil
}

func validateVaultPathSpelling(path string) error {
	if strings.HasPrefix(path, `\\?\`) || strings.HasPrefix(path, `\\.\`) {
		return fmt.Errorf("%w: device paths are forbidden", ErrUnsafePath)
	}
	if strings.HasPrefix(path, `\\`) {
		return fmt.Errorf("%w: UNC paths are forbidden", ErrRemoteUnsupported)
	}
	volume := filepath.VolumeName(path)
	if len(volume) != 2 || volume[1] != ':' {
		return fmt.Errorf("%w: Vault must use one local drive-letter volume", ErrUnsafePath)
	}
	if strings.Contains(strings.TrimPrefix(path, volume), ":") {
		return fmt.Errorf("%w: alternate data streams are forbidden", ErrUnsafePath)
	}
	return nil
}

func classifyDriveType(driveType uint32) error {
	switch driveType {
	case windows.DRIVE_FIXED:
		return nil
	case windows.DRIVE_REMOTE:
		return ErrRemoteUnsupported
	default:
		return fmt.Errorf("%w: drive type %d is not fixed local media", ErrUnsafeMedia, driveType)
	}
}

func classifyFileSystem(name string) error {
	if !strings.EqualFold(name, "NTFS") {
		return fmt.Errorf("%w: filesystem %q is not NTFS", ErrUnsafeMedia, name)
	}
	return nil
}

func classifyHotplugInfo(info storageHotplugInfo) error {
	if info.MediaRemovable != 0 || info.MediaHotplug != 0 || info.DeviceHotplug != 0 {
		return fmt.Errorf("%w: removable or hot-plug storage is forbidden", ErrUnsafeMedia)
	}
	return nil
}

func rejectMappedDrive(path string) error {
	volume := filepath.VolumeName(path)
	pointer, err := windows.UTF16PtrFromString(volume)
	if err != nil {
		return fmt.Errorf("%w: encode drive mapping: %v", ErrRemoteUnsupported, err)
	}
	buffer := make([]uint16, 1024)
	if _, err := windows.QueryDosDevice(pointer, &buffer[0], uint32(len(buffer))); err != nil {
		return fmt.Errorf("%w: resolve drive mapping: %v", ErrRemoteUnsupported, err)
	}
	target := windows.UTF16ToString(buffer)
	lowerTarget := strings.ToLower(target)
	if strings.HasPrefix(target, `\??\`) || strings.Contains(lowerTarget, "mup") ||
		strings.Contains(lowerTarget, "lanmanredirector") {
		return fmt.Errorf("%w: mapped drives are forbidden", ErrRemoteUnsupported)
	}
	return nil
}

func driveTypeForVolume(volumePath string) (uint32, error) {
	pointer, err := windows.UTF16PtrFromString(volumePath)
	if err != nil {
		return 0, err
	}
	driveType := windows.GetDriveType(pointer)
	if driveType == windows.DRIVE_UNKNOWN || driveType == windows.DRIVE_NO_ROOT_DIR {
		return driveType, errors.New("drive type is unresolved")
	}
	return driveType, nil
}

func volumePathName(path string) (string, error) {
	pointer, err := windows.UTF16PtrFromString(longPath(path))
	if err != nil {
		return "", err
	}
	for size := uint32(512); size <= 1<<15; size *= 2 {
		buffer := make([]uint16, size)
		if err := windows.GetVolumePathName(pointer, &buffer[0], uint32(len(buffer))); err == nil {
			return windows.UTF16ToString(buffer), nil
		} else if !errors.Is(err, windows.ERROR_MORE_DATA) && !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) {
			return "", err
		}
	}
	return "", errors.New("volume path exceeds supported bound")
}

func volumeNameForMountPoint(mountPoint string) (string, error) {
	pointer, err := windows.UTF16PtrFromString(mountPoint)
	if err != nil {
		return "", err
	}
	buffer := make([]uint16, windows.MAX_PATH+1)
	if err := windows.GetVolumeNameForVolumeMountPoint(pointer, &buffer[0], uint32(len(buffer))); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buffer), nil
}

func queryHotplugInfo(volumeName string) (storageHotplugInfo, error) {
	volumeName = strings.TrimSuffix(volumeName, `\`)
	pointer, err := windows.UTF16PtrFromString(volumeName)
	if err != nil {
		return storageHotplugInfo{}, err
	}
	handle, err := windows.CreateFile(pointer, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return storageHotplugInfo{}, err
	}
	defer windows.CloseHandle(handle)
	info := storageHotplugInfo{Size: uint32(unsafe.Sizeof(storageHotplugInfo{}))}
	var returned uint32
	if err := windows.DeviceIoControl(handle, ioctlStorageGetHotplugInfo,
		nil, 0, (*byte)(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), &returned, nil); err != nil {
		return storageHotplugInfo{}, err
	}
	if returned < uint32(unsafe.Sizeof(info)) || info.Size < uint32(unsafe.Sizeof(info)) {
		return storageHotplugInfo{}, errors.New("storage hot-plug response is truncated")
	}
	return info, nil
}

func rejectCloudSyncRoot(handle windows.Handle) error {
	if err := cfGetSyncRootInfoByHandle.Find(); err != nil {
		return fmt.Errorf("%w: Cloud Files API is unavailable: %v", ErrCloudSyncUnsupported, err)
	}
	var basicInfo int64
	var returned uint32
	result, _, _ := cfGetSyncRootInfoByHandle.Call(
		uintptr(handle), cfSyncRootInfoBasic,
		uintptr(unsafe.Pointer(&basicInfo)), unsafe.Sizeof(basicInfo),
		uintptr(unsafe.Pointer(&returned)),
	)
	runtime.KeepAlive(&basicInfo)
	runtime.KeepAlive(&returned)
	return classifyCloudSyncHRESULT(uint32(result))
}

func classifyCloudSyncHRESULT(result uint32) error {
	switch result {
	case 0:
		return ErrCloudSyncUnsupported
	// Cloud Files reports NOT_UNDER_SYNC_ROOT on attached volumes and may
	// report INVALID_FUNCTION where the cloud filter is not attached. A
	// registered sync root cannot exist on such a volume. Every other failure
	// remains unresolved and is rejected below.
	case hresultInvalidFunction, hresultCloudNotUnderSyncRoot, hresultNotACloudSyncRoot:
		return nil
	default:
		return fmt.Errorf("%w: Cloud Files classification failed with HRESULT 0x%08x", ErrCloudSyncUnsupported, result)
	}
}

func finalHandlePath(handle windows.Handle) (string, error) {
	size := uint32(512)
	for {
		buffer := make([]uint16, size)
		n, err := windows.GetFinalPathNameByHandle(handle, &buffer[0], uint32(len(buffer)), 0)
		if err != nil {
			return "", err
		}
		if n < uint32(len(buffer)) {
			return windows.UTF16ToString(buffer[:n]), nil
		}
		size = n + 1
	}
}

func normalizeFinalPath(path string) string {
	switch {
	case strings.HasPrefix(path, `\\?\UNC\`):
		path = `\\` + strings.TrimPrefix(path, `\\?\UNC\`)
	case strings.HasPrefix(path, `\\?\`):
		path = strings.TrimPrefix(path, `\\?\`)
	}
	return filepath.Clean(path)
}

func longPath(path string) string {
	if strings.HasPrefix(path, `\\?\`) {
		return path
	}
	if strings.HasPrefix(path, `\\`) {
		return `\\?\UNC\` + strings.TrimPrefix(path, `\\`)
	}
	return `\\?\` + path
}

// Windows does not expose a portable fsync-equivalent for a directory entry.
// File handles used by later storage layers must flush their own data; this
// function intentionally makes no claim that the mkdir survives sudden power
// loss merely because Open returned.
func syncCreatedDirectory(string) error { return nil }
