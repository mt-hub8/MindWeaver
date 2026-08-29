//go:build windows

package backup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"github.com/mt-hub8/MindWeaver/v2/internal/vault"
	"golang.org/x/sys/windows"
)

const maxWindowsFinalPathCodeUnits = 32 << 10

func hasPlatformReservedPrefix(name, prefix string) bool {
	return len(name) >= len(prefix) && strings.EqualFold(name[:len(prefix)], prefix)
}

func validPlatformDestinationInput(raw string) bool {
	if len(raw) < 4 || !asciiDriveLetter(raw[0]) || raw[1] != ':' || raw[2] != '\\' ||
		strings.ContainsRune(raw, '/') || !filepath.IsAbs(raw) || filepath.Clean(raw) != raw {
		return false
	}
	components := strings.Split(raw[3:], `\`)
	if len(components) == 0 {
		return false
	}
	for _, component := range components {
		if !validResidueLeaf(component) {
			return false
		}
	}
	return validDestinationLeaf(components[len(components)-1])
}

// validatePlatformDestinationNamespace rejects remote and unresolved drive
// mappings before any directory or file handle is opened.
func validatePlatformDestinationNamespace(raw string) error {
	if !validPlatformDestinationInput(raw) {
		return errors.New("backup: invalid Windows destination namespace")
	}
	return validatePlatformDirectoryNamespace(filepath.Dir(raw))
}

func validatePlatformDirectoryNamespace(raw string) error {
	abs, err := filepath.Abs(raw)
	if err != nil {
		return fmt.Errorf("backup: resolve Windows directory namespace: %w", err)
	}
	abs = filepath.Clean(abs)
	if len(abs) < 3 || !asciiDriveLetter(abs[0]) || abs[1] != ':' || abs[2] != '\\' ||
		strings.HasPrefix(abs, `\\`) || strings.ContainsRune(abs, '/') {
		return errors.New("backup: invalid Windows directory namespace")
	}
	drive := abs[:2]
	pointer, err := windows.UTF16PtrFromString(drive)
	if err != nil {
		return fmt.Errorf("%w: encode destination drive mapping", vault.ErrRemoteUnsupported)
	}
	buffer := make([]uint16, 1024)
	if _, err := windows.QueryDosDevice(pointer, &buffer[0], uint32(len(buffer))); err != nil {
		return fmt.Errorf("%w: resolve destination drive mapping", vault.ErrRemoteUnsupported)
	}
	if err := classifyWindowsDestinationMapping(windows.UTF16ToString(buffer)); err != nil {
		return err
	}
	rootPointer, err := windows.UTF16PtrFromString(drive + `\`)
	if err != nil {
		return fmt.Errorf("%w: encode destination drive root", vault.ErrUnsafeMedia)
	}
	switch driveType := windows.GetDriveType(rootPointer); driveType {
	case windows.DRIVE_FIXED:
	case windows.DRIVE_REMOTE:
		return vault.ErrRemoteUnsupported
	default:
		return fmt.Errorf("%w: destination drive type %d is not fixed local media", vault.ErrUnsafeMedia, driveType)
	}
	return vault.ValidateRegisteredWindowsVolumePath(abs)
}

// retainPlatformDirectoryNamespace walks a drive-rooted path one component at
// a time with OBJ_DONT_REPARSE and FILE_OPEN_REPARSE_POINT. Every directory
// handle is kept open without delete sharing for the retainedDirectory
// lifetime, so no already-validated ancestor can be renamed or replaced before
// an os.Root or an absolute-path-only durability handle is opened.
func retainPlatformDirectoryNamespace(path string) ([]*os.File, os.FileInfo, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, nil, err
	}
	abs = filepath.Clean(abs)
	if err := validatePlatformDirectoryNamespace(abs); err != nil {
		return nil, nil, err
	}
	drive := abs[:2]
	rootPath := drive + `\`
	rootPointer, err := windows.UTF16PtrFromString(longWindowsPath(rootPath))
	if err != nil {
		return nil, nil, err
	}
	rootHandle, err := windows.CreateFile(
		rootPointer,
		windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("backup: retain Windows volume root: %w", err)
	}
	rootFile := os.NewFile(uintptr(rootHandle), rootPath)
	if rootFile == nil {
		_ = windows.CloseHandle(rootHandle)
		return nil, nil, errors.New("backup: adopt retained Windows volume root")
	}
	witnesses := []*os.File{rootFile}
	fail := func(cause error) ([]*os.File, os.FileInfo, error) {
		return nil, nil, errors.Join(cause, closeNamespaceWitnesses(witnesses))
	}
	if err := validateWindowsNamespaceDirectory(rootFile); err != nil {
		return fail(err)
	}
	currentPath := rootPath
	remainder := strings.TrimPrefix(abs, rootPath)
	depth := 0
	for _, component := range strings.Split(remainder, `\`) {
		if component == "" {
			continue
		}
		if depth >= maxExactTreeDepth {
			return fail(fmt.Errorf("backup: Windows directory depth exceeds limit %d", maxExactTreeDepth))
		}
		depth++
		if !validPlatformLeaf(component) || component == "." || component == ".." {
			return fail(errors.New("backup: invalid Windows directory component"))
		}
		currentPath = filepath.Join(currentPath, component)
		child, openErr := openWindowsNamespaceChild(witnesses[len(witnesses)-1], component, currentPath)
		if openErr != nil {
			return fail(openErr)
		}
		witnesses = append(witnesses, child)
	}
	final := witnesses[len(witnesses)-1]
	info, err := final.Stat()
	if err != nil {
		return fail(err)
	}
	finalPath, err := retainedFinalPath(windows.Handle(final.Fd()))
	if err != nil {
		return fail(err)
	}
	resolved := normalizeRetainedFinalPath(finalPath)
	if strings.HasPrefix(resolved, `\\`) ||
		!strings.EqualFold(filepath.VolumeName(resolved), filepath.VolumeName(abs)) {
		return fail(errors.New("backup: retained Windows namespace resolved outside its local volume"))
	}
	return witnesses, info, nil
}

func openWindowsNamespaceChild(parent *os.File, name, displayPath string) (*os.File, error) {
	if parent == nil {
		return nil, errors.New("backup: Windows namespace parent is unavailable")
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
		return nil, fmt.Errorf("backup: retain Windows directory component: %w", err)
	}
	file := os.NewFile(uintptr(handle), displayPath)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("backup: adopt retained Windows directory component")
	}
	if err := validateWindowsNamespaceDirectory(file); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return file, nil
}

func validateWindowsNamespaceDirectory(file *os.File) error {
	if file == nil {
		return errors.New("backup: Windows namespace directory is unavailable")
	}
	var information windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &information); err != nil {
		return err
	}
	if information.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		information.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return errors.Join(vault.ErrUnsafePath, errors.New("backup: Windows namespace component is reparse or non-directory"))
	}
	return nil
}

func classifyWindowsDestinationMapping(target string) error {
	lower := strings.ToLower(target)
	if strings.HasPrefix(lower, `\device\harddiskvolume`) ||
		strings.HasPrefix(lower, `\device\harddiskdmvolumes\`) ||
		strings.HasPrefix(lower, `\device\volume{`) {
		return nil
	}
	if strings.Contains(lower, `\device\mup`) ||
		strings.Contains(lower, `\device\lanmanredirector`) ||
		strings.HasPrefix(lower, `\??\unc\`) || strings.HasPrefix(lower, `\\`) {
		return vault.ErrRemoteUnsupported
	}
	return fmt.Errorf("%w: destination drive mapping is not an approved local volume", vault.ErrUnsafeMedia)
}

func validateRetainedDirectoryCaseSemantics(directory *retainedDirectory) (resultErr error) {
	if directory == nil || directory.root == nil {
		return errors.New("backup: destination directory capability is unavailable")
	}
	file, err := directory.root.Open(".")
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, file.Close()) }()
	var flags uint32
	if err := windows.GetFileInformationByHandleEx(
		windows.Handle(file.Fd()), windows.FileCaseSensitiveInfo,
		(*byte)(unsafe.Pointer(&flags)), uint32(unsafe.Sizeof(flags)),
	); err != nil {
		return fmt.Errorf("backup: query destination case semantics: %w", err)
	}
	if flags&windows.FILE_CS_FLAG_CASE_SENSITIVE_DIR != 0 {
		return errors.New("backup: case-sensitive destination directories are unsupported")
	}
	return nil
}

func asciiDriveLetter(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func retainedManagedDestinationAncestor(directory *retainedDirectory) (bool, error) {
	if directory == nil || directory.root == nil || directory.identity.info == nil {
		return false, errors.New("backup: destination ancestor capability is unavailable")
	}
	probe, err := directory.root.Open(".")
	if err != nil {
		return false, err
	}
	path, pathErr := retainedFinalPath(windows.Handle(probe.Fd()))
	closeErr := probe.Close()
	if err := errors.Join(pathErr, closeErr); err != nil {
		return false, err
	}
	return managedDestinationLeaf(filepath.Base(normalizeRetainedFinalPath(path))), nil
}

func retainedFinalPath(handle windows.Handle) (string, error) {
	size := uint32(512)
	for size <= maxWindowsFinalPathCodeUnits {
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
	return "", errors.New("backup: destination ancestor final path exceeds limit")
}

func normalizeRetainedFinalPath(path string) string {
	switch {
	case strings.HasPrefix(path, `\\?\UNC\`):
		path = `\\` + strings.TrimPrefix(path, `\\?\UNC\`)
	case strings.HasPrefix(path, `\\?\`):
		path = strings.TrimPrefix(path, `\\?\`)
	}
	return filepath.Clean(path)
}

func validPlatformLeaf(name string) bool {
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") || strings.ContainsAny(name, `<>:"|?*`) {
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
	switch stem {
	case "CON", "PRN", "AUX", "NUL", "CLOCK$", "CONIN$", "CONOUT$",
		"COM¹", "COM²", "COM³", "LPT¹", "LPT²", "LPT³":
		return false
	}
	if len(stem) == 4 && stem[3] >= '1' && stem[3] <= '9' &&
		(stem[:3] == "COM" || stem[:3] == "LPT") {
		return false
	}
	return true
}
