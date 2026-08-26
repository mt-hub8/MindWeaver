//go:build windows

package backup

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unsafe"

	"github.com/mt-hub8/MindWeaver/v2/internal/vault"
	"golang.org/x/sys/windows"
)

type temporaryDOSDeviceAlias struct {
	drive  string
	target string
	active bool
	mutex  windows.Handle
}

func createTemporaryDirectVolumeAlias() (result *temporaryDOSDeviceAlias, resultErr error) {
	mutex, err := acquireTemporaryDOSDeviceMutex()
	if err != nil {
		return nil, err
	}
	releaseMutex := true
	defer func() {
		if releaseMutex {
			resultErr = errors.Join(resultErr, releaseTemporaryDOSDeviceMutex(mutex))
		}
	}()

	base := filepath.Clean(os.TempDir())
	sourceDrive := filepath.VolumeName(base)
	if len(sourceDrive) != 2 {
		return nil, fmt.Errorf("temporary directory has no drive-letter volume: %q", base)
	}
	sourcePointer, err := windows.UTF16PtrFromString(sourceDrive)
	if err != nil {
		return nil, err
	}
	sourceBuffer := make([]uint16, 1024)
	if _, err := windows.QueryDosDevice(sourcePointer, &sourceBuffer[0], uint32(len(sourceBuffer))); err != nil {
		return nil, fmt.Errorf("query source DOS device: %w", err)
	}
	target := windows.UTF16ToString(sourceBuffer)
	if err := classifyWindowsDestinationMapping(target); err != nil {
		return nil, fmt.Errorf("temporary directory drive is not a direct approved volume: %w", err)
	}

	logicalDrives, err := windows.GetLogicalDrives()
	if err != nil {
		return nil, fmt.Errorf("enumerate logical drives: %w", err)
	}
	var drive string
	for letter := byte('Z'); letter >= 'E'; letter-- {
		mask := uint32(1) << (letter - 'A')
		if logicalDrives&mask != 0 {
			continue
		}
		candidate := string(letter) + ":"
		candidatePointer, pointerErr := windows.UTF16PtrFromString(candidate)
		if pointerErr != nil {
			return nil, pointerErr
		}
		candidateBuffer := make([]uint16, 16)
		_, queryErr := windows.QueryDosDevice(candidatePointer, &candidateBuffer[0], uint32(len(candidateBuffer)))
		if !errors.Is(queryErr, windows.ERROR_FILE_NOT_FOUND) {
			continue
		}
		if _, statErr := os.Lstat(candidate + `\`); !errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		drive = candidate
		break
	}
	if drive == "" {
		return nil, errors.New("no provably unused DOS drive letter is available")
	}
	drivePointer, err := windows.UTF16PtrFromString(drive)
	if err != nil {
		return nil, err
	}
	targetPointer, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return nil, err
	}
	if err := windows.DefineDosDevice(
		windows.DDD_RAW_TARGET_PATH|windows.DDD_NO_BROADCAST_SYSTEM,
		drivePointer,
		targetPointer,
	); err != nil {
		return nil, fmt.Errorf("create temporary DOS device alias: %w", err)
	}
	alias := &temporaryDOSDeviceAlias{drive: drive, target: target, active: true, mutex: mutex}
	releaseMutex = false
	verifyBuffer := make([]uint16, 1024)
	if _, err := windows.QueryDosDevice(drivePointer, &verifyBuffer[0], uint32(len(verifyBuffer))); err != nil {
		return alias, fmt.Errorf("verify temporary DOS device alias: %w", err)
	}
	if mapped := windows.UTF16ToString(verifyBuffer); !strings.EqualFold(mapped, target) {
		return alias, fmt.Errorf("temporary DOS device target = %q, want %q", mapped, target)
	}
	return alias, nil
}

func acquireTemporaryDOSDeviceMutex() (windows.Handle, error) {
	// Windows mutex ownership is thread-affine. Keep this test goroutine on the
	// acquiring thread until the alias has been removed and the mutex released.
	runtime.LockOSThread()
	name, err := windows.UTF16PtrFromString(`Local\MindWeaver-DefineDosDevice-Qualification-v1`)
	if err != nil {
		runtime.UnlockOSThread()
		return 0, err
	}
	handle, err := windows.CreateMutex(nil, false, name)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if handle != 0 {
			_ = windows.CloseHandle(handle)
		}
		runtime.UnlockOSThread()
		return 0, fmt.Errorf("create DOS device qualification mutex: %w", err)
	}
	event, err := windows.WaitForSingleObject(handle, 30_000)
	if err != nil {
		_ = windows.CloseHandle(handle)
		runtime.UnlockOSThread()
		return 0, fmt.Errorf("wait for DOS device qualification mutex: %w", err)
	}
	switch event {
	case windows.WAIT_OBJECT_0:
		return handle, nil
	case windows.WAIT_ABANDONED:
		err := errors.Join(
			errors.New("DOS device qualification mutex was abandoned; prior alias cleanup is unresolved"),
			windows.ReleaseMutex(handle), windows.CloseHandle(handle),
		)
		runtime.UnlockOSThread()
		return 0, err
	case uint32(windows.WAIT_TIMEOUT):
		err := errors.Join(
			errors.New("timed out waiting for DOS device qualification mutex"),
			windows.CloseHandle(handle),
		)
		runtime.UnlockOSThread()
		return 0, err
	default:
		err := errors.Join(
			fmt.Errorf("unexpected DOS device qualification mutex result: %d", event),
			windows.CloseHandle(handle),
		)
		runtime.UnlockOSThread()
		return 0, err
	}
}

func releaseTemporaryDOSDeviceMutex(handle windows.Handle) error {
	if handle == 0 {
		return nil
	}
	err := errors.Join(windows.ReleaseMutex(handle), windows.CloseHandle(handle))
	runtime.UnlockOSThread()
	return err
}

func (alias *temporaryDOSDeviceAlias) Close() (resultErr error) {
	if alias == nil || !alias.active {
		return nil
	}
	drivePointer, err := windows.UTF16PtrFromString(alias.drive)
	if err != nil {
		return err
	}
	targetPointer, err := windows.UTF16PtrFromString(alias.target)
	if err != nil {
		return err
	}
	if err := windows.DefineDosDevice(
		windows.DDD_RAW_TARGET_PATH|windows.DDD_REMOVE_DEFINITION|
			windows.DDD_EXACT_MATCH_ON_REMOVE|windows.DDD_NO_BROADCAST_SYSTEM,
		drivePointer,
		targetPointer,
	); err != nil {
		return fmt.Errorf("remove temporary DOS device alias: %w", err)
	}
	alias.active = false
	defer func() {
		resultErr = errors.Join(resultErr, releaseTemporaryDOSDeviceMutex(alias.mutex))
		alias.mutex = 0
	}()
	verifyBuffer := make([]uint16, 16)
	if _, err := windows.QueryDosDevice(drivePointer, &verifyBuffer[0], uint32(len(verifyBuffer))); !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return fmt.Errorf("temporary DOS device alias remains queryable: %v", err)
	}
	logicalDrives, err := windows.GetLogicalDrives()
	if err != nil {
		return fmt.Errorf("verify logical drives after alias removal: %w", err)
	}
	mask := uint32(1) << (alias.drive[0] - 'A')
	if logicalDrives&mask != 0 {
		return errors.New("temporary DOS device alias remains a logical drive")
	}
	if _, err := os.Lstat(alias.drive + `\`); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("temporary DOS device alias remains filesystem-visible: %v", err)
	}
	return nil
}

func TestWindowsSessionLocalDirectVolumeAliasIsRejectedBeforeVaultOrBackupWrite(t *testing.T) {
	base := t.TempDir()
	if err := vault.ValidateRegisteredWindowsVolumePath(base); err != nil {
		t.Fatalf("registered temporary-directory drive rejected: %v", err)
	}
	if err := validatePlatformDirectoryNamespace(base); err != nil {
		t.Fatalf("registered backup directory rejected: %v", err)
	}

	alias, err := createTemporaryDirectVolumeAlias()
	if alias != nil {
		t.Cleanup(func() {
			if err := alias.Close(); err != nil {
				t.Errorf("clean temporary DOS device alias: %v", err)
			}
		})
	}
	if err != nil {
		t.Fatal(err)
	}
	sourceRoot := filepath.VolumeName(base) + `\`
	relative, err := filepath.Rel(sourceRoot, base)
	if err != nil {
		t.Fatal(err)
	}
	aliasPath := filepath.Join(alias.drive+`\`, relative)
	sourceInfo, err := os.Stat(base)
	if err != nil {
		t.Fatal(err)
	}
	aliasInfo, err := os.Stat(aliasPath)
	if err != nil {
		t.Fatalf("open temporary DOS device alias: %v", err)
	}
	if !os.SameFile(sourceInfo, aliasInfo) {
		t.Fatal("temporary DOS device alias does not identify its source directory")
	}

	opened, err := vault.Open(aliasPath)
	if opened != nil {
		_ = opened.Close()
		t.Fatal("Vault opened through a session-local direct-volume alias")
	}
	if !errors.Is(err, vault.ErrUnsafeMedia) {
		t.Fatalf("Vault alias error = %v, want ErrUnsafeMedia", err)
	}
	if err := validatePlatformDirectoryNamespace(aliasPath); !errors.Is(err, vault.ErrUnsafeMedia) {
		t.Fatalf("backup alias error = %v, want ErrUnsafeMedia", err)
	}
	destination := filepath.Join(aliasPath, "must-not-exist")
	if target, err := newDestination(destination); err == nil {
		_ = target.parent.Close()
		t.Fatal("backup destination accepted a session-local direct-volume alias")
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backup alias admission wrote destination state: %v", err)
	}
	for _, name := range []string{".mindweaver.lock", "data", "blobs"} {
		if _, err := os.Lstat(filepath.Join(base, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("Vault alias admission wrote %q: %v", name, err)
		}
	}
	if err := alias.Close(); err != nil {
		t.Fatalf("clean temporary DOS device alias: %v", err)
	}
}

func TestWindowsLeafValidationRejectsAliasesDevicesAndADS(t *testing.T) {
	invalid := []string{
		"foo:ads", "trailing.", "trailing ", "control\x1f", "CON", "con.txt", "NUL.json",
		"PRN", "AUX.data", "COM1", "com9.log", "LPT1", "lpt9.txt", "CLOCK$", "CONIN$",
		"COM¹", "com².txt", "LPT³.log",
		`quote"name`, "less<name", "greater>name", "pipe|name", "question?name", "star*name",
	}
	parent := t.TempDir()
	for _, name := range invalid {
		t.Run(name, func(t *testing.T) {
			if validResidueLeaf(name) {
				t.Fatalf("invalid Windows leaf accepted: %q", name)
			}
			if target, err := newDestination(filepath.Join(parent, name)); err == nil {
				_ = target.parent.Close()
				t.Fatalf("invalid Windows destination accepted: %q", name)
			}
			receipt := residueReceipt{
				Version: residueReceiptVersion, ID: "33333333333333333333333333333333",
				ParentIdentity: "parent", Kind: "backup",
				StagingName: stagingPrefix + "33333333333333333333333333333333", DestinationName: name,
			}
			receipt.Revision, _ = residueReceiptRevision(receipt)
			if err := validateResidueReceipt(receipt); err == nil {
				t.Fatalf("invalid receipt destination accepted: %q", name)
			}
		})
	}

	for _, name := range []string{"concrete", "com10", "auxiliary", "report.txt", ".hidden"} {
		if !validResidueLeaf(name) {
			t.Fatalf("valid Windows leaf rejected: %q", name)
		}
	}
}

func TestWindowsDestinationInputRejectsRemoteDeviceAndAliasSyntaxBeforeOpen(t *testing.T) {
	for _, raw := range []string{
		`\\attacker.invalid\share\credential-canary`,
		`\\127.0.0.1\share\loopback-canary`,
		`\\?\C:\device\target`,
		`\\.\C:\device\target`,
		`C:drive-relative`,
		`\rooted-without-drive`,
		`C:/forward/slash`,
		`C:\parent\..\target`,
		`C:\parent.\target`,
		`C:\parent :stream\target`,
	} {
		t.Run(raw, func(t *testing.T) {
			if validPlatformDestinationInput(raw) {
				t.Fatalf("unsafe Windows destination input accepted: %q", raw)
			}
			if target, err := newDestination(raw); err == nil {
				_ = target.parent.Close()
				t.Fatalf("unsafe Windows destination reached filesystem admission: %q", raw)
			}
		})
	}
}

func TestWindowsDestinationMappingClassification(t *testing.T) {
	for _, target := range []string{
		`\Device\HarddiskVolume4`,
		`\Device\HarddiskDmVolumes\PhysicalDmVolumes\BlockVolume1`,
		`\Device\Volume{01234567-89ab-cdef-0123-456789abcdef}`,
	} {
		if err := classifyWindowsDestinationMapping(target); err != nil {
			t.Fatalf("local mapping %q rejected: %v", target, err)
		}
	}
	for _, target := range []string{
		`\Device\Mup\server\share`,
		`\Device\LanmanRedirector\;Z:000000000000\server\share`,
		`\??\UNC\server\share`,
		`\??\C:\redirected`,
		`\Device\UnknownProvider\target`,
		``,
	} {
		if err := classifyWindowsDestinationMapping(target); err == nil {
			t.Fatalf("unsafe or unresolved mapping %q accepted", target)
		}
	}
}

func TestOpenRetainedDirectoryRejectsCaseSensitiveParentWhenAvailable(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "case-sensitive-parent")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	pointer, err := windows.UTF16PtrFromString(longWindowsPath(parent))
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(
		pointer,
		windows.FILE_READ_ATTRIBUTES|windows.FILE_WRITE_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		t.Skipf("BLOCKED: cannot open case-sensitive-directory control handle: %v", err)
	}
	flags := uint32(windows.FILE_CS_FLAG_CASE_SENSITIVE_DIR)
	if err := windows.SetFileInformationByHandle(
		handle, windows.FileCaseSensitiveInfo, (*byte)(unsafe.Pointer(&flags)), uint32(unsafe.Sizeof(flags)),
	); err != nil {
		t.Skipf("BLOCKED: per-directory case sensitivity is unavailable: %v", err)
	}
	t.Cleanup(func() {
		flags := uint32(0)
		restoreErr := windows.SetFileInformationByHandle(
			handle, windows.FileCaseSensitiveInfo, (*byte)(unsafe.Pointer(&flags)), uint32(unsafe.Sizeof(flags)),
		)
		closeErr := windows.CloseHandle(handle)
		if err := errors.Join(restoreErr, closeErr); err != nil {
			t.Errorf("restore directory case semantics and close handle: %v", err)
		}
	})
	if retained, err := openRetainedDirectory(parent); err == nil {
		_ = retained.Close()
		t.Fatal("case-sensitive destination parent was accepted")
	}
}

func TestOpenRetainedDirectoryPinsEveryNamespaceAncestorUntilClose(t *testing.T) {
	base := t.TempDir()
	ancestor := filepath.Join(base, "ancestor")
	target := filepath.Join(ancestor, "child", "target")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	retained, err := openRetainedDirectory(target)
	if err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(base, "moved-ancestor")
	if err := os.Rename(ancestor, moved); !errors.Is(err, windows.ERROR_SHARING_VIOLATION) &&
		!errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		_ = retained.Close()
		t.Fatalf("ancestor rename while retained = %v, want sharing/access denial", err)
	}
	if err := retained.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(ancestor, moved); err != nil {
		t.Fatalf("ancestor rename after close: %v", err)
	}
}

func TestOpenRetainedDirectoryBlocksAncestorSwapBeforeOrdinaryRootOpen(t *testing.T) {
	base := t.TempDir()
	ancestor := filepath.Join(base, "race-ancestor")
	target := filepath.Join(ancestor, "child", "target")
	canary := filepath.Join(base, "replacement-canary")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(canary, "child", "target"), 0o700); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(base, "moved-race-ancestor")
	var renameErr error
	retained, err := openRetainedDirectoryWithHooks(target, retainedDirectoryHooks{
		afterNamespaceRetained: func() error {
			renameErr = os.Rename(ancestor, moved)
			if renameErr == nil {
				output, linkErr := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", ancestor, canary).CombinedOutput()
				return fmt.Errorf("ancestor replacement unexpectedly reached junction creation: %w (%s)", linkErr, output)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("retain after blocked namespace swap: %v", err)
	}
	defer retained.Close()
	if !errors.Is(renameErr, windows.ERROR_SHARING_VIOLATION) &&
		!errors.Is(renameErr, windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("ancestor swap error = %v, want sharing/access denial", renameErr)
	}
	targetInfo, statErr := os.Lstat(target)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if !os.SameFile(retained.identity.info, targetInfo) {
		t.Fatal("ordinary root open escaped the retained namespace identity")
	}
}

func TestOpenRetainedDirectoryRejectsJunctionAncestorDuringNoFollowWalk(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "junction-target")
	child := filepath.Join(target, "child")
	junction := filepath.Join(base, "junction")
	if err := os.MkdirAll(child, 0o700); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", junction, target).CombinedOutput()
	if err != nil {
		t.Skipf("junction creation unavailable: %v (%s)", err, output)
	}
	if retained, err := openRetainedDirectory(filepath.Join(junction, "child")); err == nil {
		_ = retained.Close()
		t.Fatal("junction ancestor was accepted by retained namespace walk")
	}
}

func TestWindowsDestinationRejectsCaseVariantReservedPrefixes(t *testing.T) {
	for _, name := range []string{
		strings.ToUpper(stagingPrefix) + "canary",
		strings.ToUpper(restorePrefix) + "canary",
		strings.ToUpper(verifyScratchPrefix) + "canary",
		strings.ToUpper(residueReceiptPrefix) + "canary",
		strings.ToUpper(residueTempPrefix) + "canary",
		strings.ToUpper(startupVerifyScratchLeaf),
		strings.ToUpper(liveVerifyScratchPrefix) + strings.Repeat("a", 64),
	} {
		if validDestinationLeaf(name) {
			t.Fatalf("case-variant managed prefix accepted as destination: %q", name)
		}
	}
}

func TestCreateRejectsManagedAncestorShortPathAliasWhenAvailable(t *testing.T) {
	fixture := newBackupFixture(t)
	managed := filepath.Join(fixture.root, stagingPrefix+"long-managed-alias-parent")
	if err := os.Mkdir(managed, 0o700); err != nil {
		t.Fatal(err)
	}
	wide, err := windows.UTF16PtrFromString(managed)
	if err != nil {
		t.Fatal(err)
	}
	required, err := windows.GetShortPathName(wide, nil, 0)
	if err != nil || required == 0 {
		t.Logf("BLOCKED: Windows 8.3 managed-ancestor alias unavailable: %v", err)
		return
	}
	buffer := make([]uint16, required+1)
	if _, err := windows.GetShortPathName(wide, &buffer[0], uint32(len(buffer))); err != nil {
		t.Logf("BLOCKED: Windows 8.3 managed-ancestor alias unavailable: %v", err)
		return
	}
	shortPath := windows.UTF16ToString(buffer)
	if filepath.Clean(shortPath) == filepath.Clean(managed) {
		t.Log("BLOCKED: Windows 8.3 aliases are disabled on this volume")
		return
	}
	destination := filepath.Join(shortPath, "must-not-be-deleted-later")
	if _, err := fixture.coordinator.Create(t.Context(), destination); err == nil {
		t.Fatal("Create accepted a short-path alias into a managed staging ancestor")
	}
	if _, err := os.Lstat(filepath.Join(managed, filepath.Base(destination))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("short-path managed ancestor received a backup: %v", err)
	}
}
