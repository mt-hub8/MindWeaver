//go:build windows

package vault

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsLockHandleIdentityIsVerified(t *testing.T) {
	base := t.TempDir()
	first, err := Open(filepath.Join(base, "first"))
	if err != nil {
		t.Fatalf("Open first: %v", err)
	}
	defer first.Close()
	second, err := Open(filepath.Join(base, "second"))
	if err != nil {
		t.Fatalf("Open second: %v", err)
	}
	defer second.Close()

	firstLock := filepath.Join(first.Paths().Root, lockFileName)
	if err := verifyHandlePath(windows.Handle(first.lockFile.Fd()), firstLock, false); err != nil {
		t.Fatalf("verify actual lock path: %v", err)
	}
	wrongLock := filepath.Join(second.Paths().Root, lockFileName)
	if err := verifyHandlePath(windows.Handle(first.lockFile.Fd()), wrongLock, false); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("verify wrong lock path error = %v, want ErrUnsafePath", err)
	}
}

func TestWindowsRootIdentityUsesVolumeAndFileID(t *testing.T) {
	v, err := Open(filepath.Join(t.TempDir(), "identity-vault"))
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	identity, err := fileIdentity(windows.Handle(v.rootFile.Fd()))
	if err != nil {
		t.Fatalf("root FileIdInfo: %v", err)
	}
	if identity.VolumeSerialNumber == 0 || identity.FileIDInfo.VolumeSerialNumber == 0 ||
		identity.FileIDInfo.FileID == ([16]byte{}) {
		t.Fatalf("incomplete root identity: %#v", identity)
	}
	upperAlias := strings.ToUpper(v.Paths().Root)
	if err := verifyHandlePath(windows.Handle(v.rootFile.Fd()), upperAlias, true); err != nil {
		t.Fatalf("case-only display alias changed handle identity: %v", err)
	}
}

func TestWindowsVaultLocationClassifiersFailClosed(t *testing.T) {
	if err := validateVaultPathSpelling(`\\server\share\vault`); !errors.Is(err, ErrRemoteUnsupported) {
		t.Fatalf("UNC classification = %v, want ErrRemoteUnsupported", err)
	}
	if err := validateVaultPathSpelling(`\\?\C:\vault`); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("device path classification = %v, want ErrUnsafePath", err)
	}
	if err := validateVaultPathSpelling(`C:\vault:stream`); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("ADS classification = %v, want ErrUnsafePath", err)
	}
	if err := classifyDriveType(windows.DRIVE_FIXED); err != nil {
		t.Fatalf("fixed drive rejected: %v", err)
	}
	if err := classifyDriveType(windows.DRIVE_REMOTE); !errors.Is(err, ErrRemoteUnsupported) {
		t.Fatalf("remote drive classification = %v", err)
	}
	for _, driveType := range []uint32{windows.DRIVE_UNKNOWN, windows.DRIVE_REMOVABLE, windows.DRIVE_CDROM} {
		if err := classifyDriveType(driveType); !errors.Is(err, ErrUnsafeMedia) {
			t.Fatalf("drive type %d classification = %v, want ErrUnsafeMedia", driveType, err)
		}
	}
	if err := classifyFileSystem("NTFS"); err != nil {
		t.Fatalf("NTFS rejected: %v", err)
	}
	if err := classifyFileSystem("ReFS"); !errors.Is(err, ErrUnsafeMedia) {
		t.Fatalf("ReFS classification = %v, want ErrUnsafeMedia", err)
	}
	for _, info := range []storageHotplugInfo{
		{MediaRemovable: 1},
		{MediaHotplug: 1},
		{DeviceHotplug: 1},
	} {
		if err := classifyHotplugInfo(info); !errors.Is(err, ErrUnsafeMedia) {
			t.Fatalf("hot-plug classification = %v, want ErrUnsafeMedia", err)
		}
	}
	if err := classifyCloudSyncHRESULT(0); !errors.Is(err, ErrCloudSyncUnsupported) {
		t.Fatalf("sync-root classification = %v, want ErrCloudSyncUnsupported", err)
	}
	for _, result := range []uint32{hresultInvalidFunction, hresultCloudNotUnderSyncRoot, hresultNotACloudSyncRoot} {
		if err := classifyCloudSyncHRESULT(result); err != nil {
			t.Fatalf("non-sync-root HRESULT 0x%08x rejected: %v", result, err)
		}
	}
	if err := classifyCloudSyncHRESULT(0x80004005); !errors.Is(err, ErrCloudSyncUnsupported) {
		t.Fatalf("unresolved Cloud Files result = %v, want fail-closed ErrCloudSyncUnsupported", err)
	}
}

func TestWindowsRegisteredVolumePathResponseFailsClosed(t *testing.T) {
	paths, err := parseVolumePathNames([]uint16{'D', ':', '\\', 0, 'D', ':', '\\', 'm', '\\', 0, 0})
	if err != nil {
		t.Fatalf("parse registered paths: %v", err)
	}
	if len(paths) != 2 || paths[0] != `D:\` || paths[1] != `D:\m\` {
		t.Fatalf("registered paths = %#v", paths)
	}
	for _, malformed := range [][]uint16{
		nil,
		{'D', ':', '\\'},
		{'D', ':', '\\', 0},
	} {
		if paths, err := parseVolumePathNames(malformed); err == nil {
			t.Fatalf("malformed registered paths accepted: %#v", paths)
		}
	}
}

func TestWindowsRejectsDetectedCloudFilesSyncRoot(t *testing.T) {
	oneDrive := os.Getenv("OneDrive")
	if oneDrive == "" {
		t.Skip("OneDrive environment is unavailable")
	}
	info, err := os.Lstat(oneDrive)
	if err != nil || !info.IsDir() {
		t.Skipf("OneDrive sync root is unavailable: %v", err)
	}
	handle, err := openPathHandle(oneDrive, true, true, true)
	if err != nil {
		t.Skipf("cannot inspect OneDrive sync root: %v", err)
	}
	defer windows.CloseHandle(handle)
	if err := rejectCloudSyncRoot(handle); !errors.Is(err, ErrCloudSyncUnsupported) {
		t.Fatalf("OneDrive classification = %v, want ErrCloudSyncUnsupported", err)
	}
}

func TestOpenRejectsJunctionRoot(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "junction-target")
	junction := filepath.Join(base, "junction")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", junction, target).CombinedOutput()
	if err != nil {
		t.Skipf("junction creation unavailable: %v (%s)", err, output)
	}
	if _, err := Open(junction); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("junction root error = %v, want ErrUnsafePath", err)
	}
	if _, err := Open(filepath.Join(junction, "child")); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("junction ancestor error = %v, want ErrUnsafePath", err)
	}

	fixedRoot := filepath.Join(base, "fixed-root")
	if err := os.Mkdir(fixedRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	fixedJunction := filepath.Join(fixedRoot, "data")
	output, err = exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", fixedJunction, target).CombinedOutput()
	if err != nil {
		t.Fatalf("create fixed junction: %v (%s)", err, output)
	}
	if _, err := Open(fixedRoot); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("fixed junction error = %v, want ErrUnsafePath", err)
	}
}
