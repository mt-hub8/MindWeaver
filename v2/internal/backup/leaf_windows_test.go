//go:build windows

package backup

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

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
