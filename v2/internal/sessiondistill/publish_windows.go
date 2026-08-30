//go:build windows

package sessiondistill

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	publicationStagingPrefix      = ".mindweaver-ideas-"
	maxPublicationStagingAttempts = 100
	ntFileCreated                 = 2
)

const atomicDirectoryPublicationSupported = true

var errPublishUnsupported = errors.New("atomic directory publication is unsupported on this platform")

type publicationNamespaceGuard struct {
	witnesses []*os.File
}

type retainedPublicationStaging struct {
	file      *os.File
	info      os.FileInfo
	name      string
	path      string
	children  []retainedPublicationChild
	published bool
}

type retainedPublicationChild struct {
	file         *os.File
	info         os.FileInfo
	name         string
	expectedSize int64
	expectedHash [sha256.Size]byte
}

type publicationPlatformHooks struct {
	afterStagingRetained func(*retainedPublicationStaging) error
	beforeRename         func(*retainedPublicationStaging) error
	afterChildrenClosed  func(*retainedPublicationStaging) error
}

type retainedPublishedBundle struct {
	directory *os.File
	children  []*os.File
}

func (guard *publicationNamespaceGuard) Close() error {
	if guard == nil {
		return nil
	}
	var result error
	for index := len(guard.witnesses) - 1; index >= 0; index-- {
		if guard.witnesses[index] != nil {
			result = errors.Join(result, guard.witnesses[index].Close())
		}
	}
	guard.witnesses = nil
	return result
}

func (guard *publicationNamespaceGuard) parent() (*os.File, error) {
	if guard == nil || len(guard.witnesses) == 0 || guard.witnesses[len(guard.witnesses)-1] == nil {
		return nil, errors.New("publication parent capability is unavailable")
	}
	return guard.witnesses[len(guard.witnesses)-1], nil
}

func retainPublicationParent(path string) (*publicationNamespaceGuard, error) {
	return retainPublicationParentWithHook(path, nil)
}

// retainPublicationParentWithHook retains the drive root and every existing
// directory component without delete sharing. Absolute-path operations remain
// safe while the guard is live because no validated ancestor can be renamed
// and replaced with a reparse point between validation and publication.
func retainPublicationParentWithHook(path string, afterRetained func([]*os.File) error) (*publicationNamespaceGuard, error) {
	if strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, `\\?\`) || strings.HasPrefix(path, `\\.\`) {
		return nil, errors.New("publication parent must be local")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	absolute = filepath.Clean(absolute)
	volume := filepath.VolumeName(absolute)
	if len(volume) != 2 || volume[1] != ':' {
		return nil, errors.New("publication parent must use a drive letter")
	}
	root := strings.ToUpper(volume[:1]) + `:\`
	rootPointer, err := windows.UTF16PtrFromString(root)
	if err != nil || windows.GetDriveType(rootPointer) != windows.DRIVE_FIXED {
		return nil, errors.New("publication parent must use fixed local media")
	}
	volumeName := make([]uint16, windows.MAX_PATH+1)
	if err := windows.GetVolumeNameForVolumeMountPoint(rootPointer, &volumeName[0], uint32(len(volumeName))); err != nil {
		return nil, errors.New("publication parent drive is not registered")
	}
	rootHandle, err := windows.CreateFile(
		rootPointer,
		windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, err
	}
	rootFile := os.NewFile(uintptr(rootHandle), root)
	if rootFile == nil {
		_ = windows.CloseHandle(rootHandle)
		return nil, errors.New("adopt publication volume handle")
	}
	witnesses := []*os.File{rootFile}
	fail := func(cause error) (*publicationNamespaceGuard, error) {
		guard := &publicationNamespaceGuard{witnesses: witnesses}
		return nil, errors.Join(cause, guard.Close())
	}
	if err := validatePublicationNamespaceDirectory(rootFile); err != nil {
		return fail(err)
	}
	current := root
	remainder := strings.TrimPrefix(absolute[len(volume):], string(os.PathSeparator))
	for _, component := range strings.Split(remainder, string(os.PathSeparator)) {
		if component == "" {
			continue
		}
		if component == "." || component == ".." || strings.ContainsRune(component, os.PathSeparator) {
			return fail(errors.New("publication parent has an invalid component"))
		}
		current = filepath.Join(current, component)
		child, openErr := openPublicationNamespaceChild(witnesses[len(witnesses)-1], component, current)
		if openErr != nil {
			return fail(openErr)
		}
		witnesses = append(witnesses, child)
	}
	witnessInfo, err := witnesses[len(witnesses)-1].Stat()
	if err != nil {
		return fail(err)
	}
	if afterRetained != nil {
		if err := afterRetained(witnesses); err != nil {
			return fail(err)
		}
	}
	// A relative namespace witness detects reparse traversal and pins every
	// ancestor. Reopen the final parent through its ordinary absolute name and
	// compare identities before returning: the final handle then blocks direct
	// parent replacement, while an attacker that won the small reopen race is
	// detected instead of being adopted.
	currentInfo, err := os.Lstat(absolute)
	if err != nil || currentInfo.Mode()&os.ModeSymlink != 0 || !currentInfo.IsDir() {
		return fail(errors.Join(err, errors.New("publication parent is not a real directory")))
	}
	pointer, err := windows.UTF16PtrFromString(longWindowsPath(absolute))
	if err != nil {
		return fail(err)
	}
	finalHandle, err := windows.CreateFile(
		pointer,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return fail(err)
	}
	finalFile := os.NewFile(uintptr(finalHandle), absolute)
	if finalFile == nil {
		_ = windows.CloseHandle(finalHandle)
		return fail(errors.New("adopt publication parent handle"))
	}
	witnesses = append(witnesses, finalFile)
	finalInfo, err := finalFile.Stat()
	if err != nil || !finalInfo.IsDir() || !os.SameFile(witnessInfo, finalInfo) || !os.SameFile(currentInfo, finalInfo) {
		return fail(errors.Join(err, errors.New("publication parent identity changed while retaining namespace")))
	}
	return &publicationNamespaceGuard{witnesses: witnesses}, nil
}

func openPublicationNamespaceChild(parent *os.File, name, displayPath string) (*os.File, error) {
	if parent == nil {
		return nil, errors.New("publication namespace parent is unavailable")
	}
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, err
	}
	attributes := windows.OBJECT_ATTRIBUTES{
		Length:        uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		RootDirectory: windows.Handle(parent.Fd()),
		ObjectName:    objectName,
		Attributes:    windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	var status windows.IO_STATUS_BLOCK
	var handle windows.Handle
	err = windows.NtCreateFile(
		&handle,
		windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE,
		&attributes,
		&status,
		nil,
		windows.FILE_ATTRIBUTE_NORMAL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		windows.FILE_OPEN,
		windows.FILE_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT,
		0,
		0,
	)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), displayPath)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("adopt publication namespace handle")
	}
	if err := validatePublicationNamespaceDirectory(file); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return file, nil
}

func validatePublicationNamespaceDirectory(file *os.File) error {
	if file == nil {
		return errors.New("publication namespace directory is unavailable")
	}
	var information windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &information); err != nil ||
		information.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 ||
		information.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.Join(err, errors.New("publication namespace handle is unsafe"))
	}
	return nil
}

func publishBundlePlatform(ctx context.Context, destination string, bundle Bundle, guard *publicationNamespaceGuard) error {
	return publishBundlePlatformWithHooks(ctx, destination, bundle, guard, publicationPlatformHooks{})
}

func publishBundlePlatformWithHooks(
	ctx context.Context,
	destination string,
	bundle Bundle,
	guard *publicationNamespaceGuard,
	hooks publicationPlatformHooks,
) error {
	if err := canceled(ctx); err != nil {
		return err
	}
	parent, err := guard.parent()
	if err != nil {
		return newError(CodeOutputFailed, err)
	}
	name := filepath.Base(destination)
	if !validPublicationLeaf(name) || !strings.EqualFold(
		filepath.Clean(filepath.Dir(destination)),
		filepath.Clean(guard.witnesses[len(guard.witnesses)-1].Name()),
	) {
		return invalid()
	}
	staging, err := createRetainedPublicationStaging(parent, filepath.Dir(destination), nil)
	if err != nil {
		return newError(CodeOutputFailed, err)
	}
	cleanup := func(cause error) error {
		return errors.Join(cause, staging.Close())
	}
	if hooks.afterStagingRetained != nil {
		if err := hooks.afterStagingRetained(staging); err != nil {
			return newError(CodeOutputFailed, cleanup(err))
		}
	}
	if err := staging.writeFile("report.json", bundle.json); err != nil {
		return newError(CodeOutputFailed, cleanup(err))
	}
	if err := canceled(ctx); err != nil {
		return cleanup(err)
	}
	if err := staging.writeFile("report.md", bundle.markdown); err != nil {
		return newError(CodeOutputFailed, cleanup(err))
	}
	if err := windows.FlushFileBuffers(windows.Handle(staging.file.Fd())); err != nil {
		return newError(CodeOutputFailed, cleanup(errors.Join(errors.New("sync publication staging"), err)))
	}
	if err := canceled(ctx); err != nil {
		return cleanup(err)
	}
	if hooks.beforeRename != nil {
		if err := hooks.beforeRename(staging); err != nil {
			return newError(CodeOutputFailed, cleanup(err))
		}
	}
	if err := verifyRetainedPublicationStaging(staging); err != nil {
		return newError(CodeOutputFailed, cleanup(err))
	}
	if err := verifyRetainedPublicationChildren(staging); err != nil {
		return newError(CodeOutputFailed, cleanup(err))
	}
	if err := staging.closeChildrenForRename(); err != nil {
		return newError(CodeOutputFailed, cleanup(err))
	}
	if hooks.afterChildrenClosed != nil {
		if err := hooks.afterChildrenClosed(staging); err != nil {
			return newError(CodeOutputFailed, cleanup(err))
		}
	}
	if err := renameRetainedPublicationStaging(staging, parent, name); err != nil {
		err = cleanup(errors.Join(errors.New("rename retained publication staging"), err))
		if errors.Is(err, os.ErrExist) {
			return newError(CodeDestinationExists, err)
		}
		return newError(CodeOutputFailed, err)
	}
	staging.published = true
	published, verifyErr := retainVerifiedPublishedBundle(parent, name, destination, staging)
	stagingCloseErr := staging.Close()
	if verifyErr != nil {
		return newError(CodePublicationUncertain, errors.Join(verifyErr, stagingCloseErr))
	}
	if stagingCloseErr != nil {
		return newError(CodePublicationUncertain, errors.Join(stagingCloseErr, published.Close()))
	}
	syncErr := syncPublishedDirectory(filepath.Dir(destination))
	closeErr := published.Close()
	if err := errors.Join(syncErr, closeErr); err != nil {
		return newError(CodePublicationUncertain, err)
	}
	return nil
}

func createRetainedPublicationStaging(
	parent *os.File,
	parentPath string,
	afterCreated func(*os.File, os.FileInfo, string) error,
) (*retainedPublicationStaging, error) {
	if parent == nil {
		return nil, errors.New("publication parent capability is unavailable")
	}
	for range maxPublicationStagingAttempts {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return nil, err
		}
		name := publicationStagingPrefix + hex.EncodeToString(random[:])
		staging, err := createRetainedPublicationStagingNamed(parent, parentPath, name, afterCreated)
		if err == nil {
			return staging, nil
		}
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return nil, err
	}
	return nil, os.ErrExist
}

func createRetainedPublicationStagingNamed(
	parent *os.File,
	parentPath string,
	name string,
	afterCreated func(*os.File, os.FileInfo, string) error,
) (*retainedPublicationStaging, error) {
	if parent == nil || !validPublicationStagingLeaf(name) {
		return nil, invalid()
	}
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, err
	}
	attributes := windows.OBJECT_ATTRIBUTES{
		Length:        uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		RootDirectory: windows.Handle(parent.Fd()),
		ObjectName:    objectName,
		Attributes:    windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	var status windows.IO_STATUS_BLOCK
	var handle windows.Handle
	err = windows.NtCreateFile(
		&handle,
		windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE|windows.SYNCHRONIZE,
		&attributes,
		&status,
		nil,
		windows.FILE_ATTRIBUTE_NORMAL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		windows.FILE_CREATE,
		windows.FILE_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_WRITE_THROUGH,
		0,
		0,
	)
	if err != nil {
		if status, ok := err.(windows.NTStatus); ok && status == windows.STATUS_OBJECT_NAME_COLLISION {
			return nil, os.ErrExist
		}
		return nil, err
	}
	failHandle := func(cause error) (*retainedPublicationStaging, error) {
		return nil, errors.Join(cause, windows.CloseHandle(handle))
	}
	if status.Information != ntFileCreated {
		return failHandle(errors.New("publication staging was not newly created"))
	}
	path := filepath.Join(parentPath, name)
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		return failHandle(errors.New("adopt publication staging handle"))
	}
	createdInfo, err := file.Stat()
	if err != nil || !createdInfo.IsDir() {
		return nil, errors.Join(err, errors.New("publication staging handle is not a directory"), file.Close())
	}
	if afterCreated != nil {
		if err := afterCreated(file, createdInfo, path); err != nil {
			return nil, errors.Join(err, file.Close())
		}
	}
	currentInfo, err := os.Lstat(path)
	if err != nil || !currentInfo.IsDir() || currentInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(createdInfo, currentInfo) {
		return nil, errors.Join(err, errors.New("publication staging identity changed before retention"), file.Close())
	}
	if err := validatePublicationNamespaceDirectory(file); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return &retainedPublicationStaging{file: file, info: createdInfo, name: name, path: path}, nil
}

func (staging *retainedPublicationStaging) writeFile(name string, content []byte) error {
	if staging == nil || staging.file == nil || (name != "report.json" && name != "report.md") {
		return errors.New("publication staging capability is unavailable")
	}
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return err
	}
	attributes := windows.OBJECT_ATTRIBUTES{
		Length:        uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		RootDirectory: windows.Handle(staging.file.Fd()),
		ObjectName:    objectName,
		Attributes:    windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	var status windows.IO_STATUS_BLOCK
	var handle windows.Handle
	err = windows.NtCreateFile(
		&handle,
		windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE|windows.SYNCHRONIZE,
		&attributes,
		&status,
		nil,
		windows.FILE_ATTRIBUTE_NORMAL,
		windows.FILE_SHARE_READ,
		windows.FILE_CREATE,
		windows.FILE_NON_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_WRITE_THROUGH,
		0,
		0,
	)
	if err != nil {
		return err
	}
	if status.Information != ntFileCreated {
		return errors.Join(errors.New("publication file was not newly created"), windows.CloseHandle(handle))
	}
	file := os.NewFile(uintptr(handle), filepath.Join(staging.path, name))
	if file == nil {
		return errors.Join(errors.New("adopt publication file handle"), windows.CloseHandle(handle))
	}
	child := retainedPublicationChild{
		file:         file,
		name:         name,
		expectedSize: int64(len(content)),
		expectedHash: sha256.Sum256(content),
	}
	staging.children = append(staging.children, child)
	childIndex := len(staging.children) - 1
	var information windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &information); err != nil ||
		information.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 ||
		information.NumberOfLinks != 1 {
		return errors.Join(err, errors.New("publication file handle is unsafe"))
	}
	fileInfo, err := file.Stat()
	if err != nil || !fileInfo.Mode().IsRegular() {
		return errors.Join(err, errors.New("publication file identity is unsafe"))
	}
	staging.children[childIndex].info = fileInfo
	written, writeErr := io.Copy(file, bytesReader(content))
	if writeErr == nil && written != int64(len(content)) {
		writeErr = io.ErrShortWrite
	}
	return errors.Join(writeErr, file.Sync())
}

func verifyRetainedPublicationStaging(staging *retainedPublicationStaging) error {
	if staging == nil || staging.file == nil || staging.info == nil {
		return errors.New("publication staging capability is unavailable")
	}
	handleInfo, err := staging.file.Stat()
	currentInfo, currentErr := os.Lstat(staging.path)
	if err := errors.Join(err, currentErr); err != nil {
		return err
	}
	if !handleInfo.IsDir() || !currentInfo.IsDir() || currentInfo.Mode()&os.ModeSymlink != 0 ||
		!os.SameFile(staging.info, handleInfo) || !os.SameFile(staging.info, currentInfo) {
		return errors.New("publication staging identity changed before rename")
	}
	return nil
}

func verifyRetainedPublicationChildren(staging *retainedPublicationStaging) error {
	if staging == nil || len(staging.children) != 2 {
		return errors.New("publication file capabilities are unavailable")
	}
	if err := verifyPublicationDirectoryEntries(staging.file); err != nil {
		return err
	}
	for index := range staging.children {
		child := &staging.children[index]
		if child.file == nil || child.info == nil {
			return errors.New("publication file capability is unavailable")
		}
		if err := verifyPublicationFile(child.file, child); err != nil {
			return errors.Join(err, errors.New("publication file changed before rename"))
		}
	}
	return nil
}

func retainVerifiedPublishedBundle(
	parent *os.File,
	destinationName string,
	destinationPath string,
	staging *retainedPublicationStaging,
) (*retainedPublishedBundle, error) {
	if parent == nil || staging == nil || staging.info == nil || len(staging.children) != 2 ||
		!validPublicationLeaf(destinationName) {
		return nil, errors.New("published bundle capability is unavailable")
	}
	directory, err := openPublishedPublicationDirectory(parent, destinationName, destinationPath)
	if err != nil {
		return nil, err
	}
	result := &retainedPublishedBundle{directory: directory}
	fail := func(cause error) (*retainedPublishedBundle, error) {
		return nil, errors.Join(cause, result.Close())
	}
	directoryInfo, err := directory.Stat()
	if err != nil || !directoryInfo.IsDir() || !os.SameFile(staging.info, directoryInfo) {
		return fail(errors.Join(err, errors.New("published directory identity changed")))
	}
	if err := verifyPublicationDirectoryEntries(directory); err != nil {
		return fail(err)
	}
	for index := range staging.children {
		expected := &staging.children[index]
		file, openErr := openPublishedPublicationFile(directory, expected.name, filepath.Join(destinationPath, expected.name))
		if openErr != nil {
			return fail(openErr)
		}
		result.children = append(result.children, file)
		if verifyErr := verifyPublicationFile(file, expected); verifyErr != nil {
			return fail(errors.Join(verifyErr, errors.New("published file changed before verification")))
		}
	}
	return result, nil
}

func openPublishedPublicationDirectory(parent *os.File, name, displayPath string) (*os.File, error) {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, err
	}
	attributes := windows.OBJECT_ATTRIBUTES{
		Length:        uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		RootDirectory: windows.Handle(parent.Fd()),
		ObjectName:    objectName,
		Attributes:    windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	var status windows.IO_STATUS_BLOCK
	var handle windows.Handle
	err = windows.NtCreateFile(
		&handle,
		windows.FILE_GENERIC_READ|windows.SYNCHRONIZE,
		&attributes,
		&status,
		nil,
		windows.FILE_ATTRIBUTE_NORMAL,
		// The retained staging handle still has DELETE access. This verification
		// handle must therefore share DELETE until that original handle closes;
		// the reopened report children deny DELETE and keep the published
		// directory from being renamed through the successful return path.
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_OPEN,
		windows.FILE_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT,
		0,
		0,
	)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), displayPath)
	if file == nil {
		return nil, errors.Join(errors.New("adopt published directory handle"), windows.CloseHandle(handle))
	}
	if err := validatePublicationNamespaceDirectory(file); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return file, nil
}

func openPublishedPublicationFile(parent *os.File, name, displayPath string) (*os.File, error) {
	if parent == nil || (name != "report.json" && name != "report.md") {
		return nil, errors.New("invalid published file capability")
	}
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, err
	}
	attributes := windows.OBJECT_ATTRIBUTES{
		Length:        uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		RootDirectory: windows.Handle(parent.Fd()),
		ObjectName:    objectName,
		Attributes:    windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	var status windows.IO_STATUS_BLOCK
	var handle windows.Handle
	err = windows.NtCreateFile(
		&handle,
		windows.FILE_GENERIC_READ|windows.SYNCHRONIZE,
		&attributes,
		&status,
		nil,
		windows.FILE_ATTRIBUTE_NORMAL,
		windows.FILE_SHARE_READ,
		windows.FILE_OPEN,
		windows.FILE_NON_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT,
		0,
		0,
	)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), displayPath)
	if file == nil {
		return nil, errors.Join(errors.New("adopt published file handle"), windows.CloseHandle(handle))
	}
	return file, nil
}

func verifyPublicationDirectoryEntries(directory *os.File) error {
	if directory == nil {
		return errors.New("publication directory capability is unavailable")
	}
	entries, err := directory.ReadDir(3)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if len(entries) != 2 {
		return errors.New("publication directory does not contain the exact report set")
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || (entry.Name() != "report.json" && entry.Name() != "report.md") || seen[entry.Name()] {
			return errors.New("publication directory contains an unexpected entry")
		}
		seen[entry.Name()] = true
	}
	if !seen["report.json"] || !seen["report.md"] {
		return errors.New("publication directory is missing a report")
	}
	return nil
}

func verifyPublicationFile(file *os.File, expected *retainedPublicationChild) error {
	if file == nil || expected == nil || expected.info == nil || expected.expectedSize < 0 {
		return errors.New("publication file capability is unavailable")
	}
	current, err := file.Stat()
	if err != nil || !current.Mode().IsRegular() || current.Size() != expected.expectedSize || !os.SameFile(expected.info, current) {
		return errors.Join(err, errors.New("publication file identity or size changed"))
	}
	var information windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &information); err != nil ||
		information.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 ||
		information.NumberOfLinks != 1 {
		return errors.Join(err, errors.New("publication file handle is unsafe"))
	}
	content, err := io.ReadAll(io.NewSectionReader(file, 0, expected.expectedSize+1))
	actualHash := sha256.Sum256(content)
	if err != nil || int64(len(content)) != expected.expectedSize || !bytes.Equal(actualHash[:], expected.expectedHash[:]) {
		return errors.Join(err, errors.New("publication file content changed"))
	}
	return nil
}

func (bundle *retainedPublishedBundle) Close() error {
	if bundle == nil {
		return nil
	}
	var result error
	for index := len(bundle.children) - 1; index >= 0; index-- {
		if bundle.children[index] != nil {
			result = errors.Join(result, bundle.children[index].Close())
		}
	}
	bundle.children = nil
	if bundle.directory != nil {
		result = errors.Join(result, bundle.directory.Close())
		bundle.directory = nil
	}
	return result
}

func renameRetainedPublicationStaging(staging *retainedPublicationStaging, parent *os.File, destinationName string) error {
	if staging == nil || staging.file == nil || parent == nil || !validPublicationLeaf(destinationName) {
		return errors.New("invalid retained publication rename")
	}
	name, err := windows.UTF16FromString(destinationName)
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
	information.RootDirectory = windows.Handle(parent.Fd())
	information.FileNameLength = uint32(len(name) * 2)
	copy(unsafe.Slice(&information.FileName[0], len(name)), name)
	err = windows.NtSetInformationFile(
		windows.Handle(staging.file.Fd()),
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

func (staging *retainedPublicationStaging) Close() error {
	if staging == nil {
		return nil
	}
	var result error
	for index := len(staging.children) - 1; index >= 0; index-- {
		child := &staging.children[index]
		if child.file != nil {
			if !staging.published {
				result = errors.Join(result, markPublicationHandleForDeletion(child.file))
			}
			result = errors.Join(result, child.file.Close())
			child.file = nil
		} else if !staging.published && child.info != nil {
			result = errors.Join(result, deleteRetainedPublicationChild(staging, *child))
		}
	}
	staging.children = nil
	if staging.file != nil {
		if !staging.published {
			result = errors.Join(result, markPublicationHandleForDeletion(staging.file))
		}
		result = errors.Join(result, staging.file.Close())
		staging.file = nil
	}
	return result
}

func (staging *retainedPublicationStaging) closeChildrenForRename() error {
	if staging == nil || staging.file == nil {
		return errors.New("publication staging capability is unavailable")
	}
	for index := range staging.children {
		child := &staging.children[index]
		if child.file == nil || child.info == nil {
			return errors.New("publication file capability is unavailable")
		}
		if err := child.file.Close(); err != nil {
			return err
		}
		child.file = nil
	}
	return nil
}

func deleteRetainedPublicationChild(staging *retainedPublicationStaging, child retainedPublicationChild) error {
	if staging == nil || staging.file == nil || child.info == nil || (child.name != "report.json" && child.name != "report.md") {
		return errors.New("invalid retained publication child cleanup")
	}
	objectName, err := windows.NewNTUnicodeString(child.name)
	if err != nil {
		return err
	}
	attributes := windows.OBJECT_ATTRIBUTES{
		Length:        uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		RootDirectory: windows.Handle(staging.file.Fd()),
		ObjectName:    objectName,
		Attributes:    windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	var status windows.IO_STATUS_BLOCK
	var handle windows.Handle
	err = windows.NtCreateFile(
		&handle,
		windows.DELETE|windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE,
		&attributes,
		&status,
		nil,
		windows.FILE_ATTRIBUTE_NORMAL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_OPEN,
		windows.FILE_NON_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT,
		0,
		0,
	)
	if err != nil {
		if status, ok := err.(windows.NTStatus); ok && status == windows.STATUS_OBJECT_NAME_NOT_FOUND {
			return nil
		}
		return err
	}
	file := os.NewFile(uintptr(handle), filepath.Join(staging.path, child.name))
	if file == nil {
		return errors.Join(errors.New("adopt publication child cleanup handle"), windows.CloseHandle(handle))
	}
	current, statErr := file.Stat()
	if statErr != nil || !current.Mode().IsRegular() || !os.SameFile(child.info, current) {
		return errors.Join(statErr, errors.New("publication child identity changed before cleanup"), file.Close())
	}
	return errors.Join(markPublicationHandleForDeletion(file), file.Close())
}

func markPublicationHandleForDeletion(file *os.File) error {
	if file == nil {
		return nil
	}
	deleteFile := byte(1)
	return windows.SetFileInformationByHandle(
		windows.Handle(file.Fd()), windows.FileDispositionInfo, &deleteFile, 1,
	)
}

func validPublicationStagingLeaf(name string) bool {
	return strings.HasPrefix(name, publicationStagingPrefix) && len(name) == len(publicationStagingPrefix)+32 && validPublicationLeaf(name)
}

func validPublicationLeaf(name string) bool {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name ||
		strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") || strings.ContainsAny(name, `<>:"/\\|?*`) {
		return false
	}
	for _, character := range name {
		if character <= 0x1f {
			return false
		}
	}
	stem := name
	if dot := strings.IndexByte(stem, '.'); dot >= 0 {
		stem = stem[:dot]
	}
	stem = strings.ToUpper(stem)
	if len(stem) == 4 && stem[3] >= '1' && stem[3] <= '9' && (stem[:3] == "COM" || stem[:3] == "LPT") {
		return false
	}
	switch stem {
	case "CON", "PRN", "AUX", "NUL", "CLOCK$", "CONIN$", "CONOUT$":
		return false
	}
	return true
}

func syncPublishedDirectory(path string) error {
	pointer, err := syscall.UTF16PtrFromString(longWindowsPath(path))
	if err != nil {
		return err
	}
	handle, err := syscall.CreateFile(
		pointer,
		syscall.GENERIC_WRITE,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return err
	}
	return errors.Join(syscall.FlushFileBuffers(handle), syscall.CloseHandle(handle))
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
