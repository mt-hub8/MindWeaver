//go:build windows

package backup

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"

	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"golang.org/x/sys/windows"
)

type savedWindowsDACL struct {
	path       string
	descriptor *windows.SECURITY_DESCRIPTOR
	daclString string
	protected  bool
}

func TestRestoreAcceptsReadExecuteOnlyWindowsBackup(t *testing.T) {
	fixture := newBackupFixture(t)
	fixture.addDocument(t, "readonly-windows", "read execute only backup restore")
	backupPath := filepath.Join(fixture.root, "readonly-backup")
	manifest, err := fixture.coordinator.Create(t.Context(), backupPath)
	if err != nil {
		t.Fatal(err)
	}

	saved, err := captureWindowsDACLTree(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := restoreWindowsDACLTree(saved); err != nil {
			t.Errorf("restore backup ACLs: %v", err)
		}
	})
	if err := applyCurrentUserReadExecuteDACL(saved); err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_PRIVILEGE_NOT_HELD) {
			t.Skipf("cannot apply a protected read/execute ACL on this runner: %v", err)
		}
		t.Fatal(err)
	}

	reader, err := openRetainedDirectory(backupPath)
	if err != nil {
		t.Fatalf("open read-only retained backup root: %v", err)
	}
	if reader.syncHandle != nil {
		_ = reader.Close()
		t.Fatal("read-only retained backup unexpectedly acquired a sync handle")
	}
	if err := reader.acquireSyncHandle(); err == nil {
		_ = reader.Close()
		t.Fatal("read-only retained backup unexpectedly upgraded to a writable sync handle")
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if handle, err := openDirectorySyncHandle(backupPath); err == nil {
		_ = handle.Close()
		t.Fatal("read/execute backup unexpectedly granted the writable sync capability")
	}
	manifestPath := filepath.Join(backupPath, manifestFileName)
	if file, err := os.OpenFile(manifestPath, os.O_WRONLY, 0); err == nil {
		_ = file.Close()
		t.Fatal("read/execute backup unexpectedly allowed opening an artifact for writing")
	}
	if _, err := readManifest(manifestPath); err != nil {
		t.Fatalf("read manifest through RX-only capability: %v", err)
	}

	destination := filepath.Join(fixture.root, "restored-from-readonly-backup")
	if err := fixture.coordinator.Restore(t.Context(), backupPath, destination); err != nil {
		t.Fatalf("restore read/execute backup: %v", err)
	}
	if err := verifyBackupTree(t.Context(), backupPath, manifest); err != nil {
		t.Fatalf("source backup changed during restore: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destination, "data", store.DatabaseFileName)); err != nil {
		t.Fatalf("restored database missing: %v", err)
	}
}

func captureWindowsDACLTree(root string) ([]savedWindowsDACL, error) {
	var saved []savedWindowsDACL
	err := filepath.WalkDir(root, func(path string, _ os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		descriptor, err := windows.GetNamedSecurityInfo(
			path,
			windows.SE_FILE_OBJECT,
			windows.DACL_SECURITY_INFORMATION,
		)
		if err != nil {
			return err
		}
		control, _, err := descriptor.Control()
		if err != nil {
			return err
		}
		saved = append(saved, savedWindowsDACL{
			path: path, descriptor: descriptor,
			daclString: descriptor.String(),
			protected:  control&windows.SE_DACL_PROTECTED != 0,
		})
		return nil
	})
	return saved, err
}

func applyCurrentUserReadExecuteDACL(saved []savedWindowsDACL) error {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		return err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return err
	}
	descriptor, err := windows.SecurityDescriptorFromString(
		"D:P(A;;GRGX;;;" + user.User.Sid.String() + ")",
	)
	if err != nil {
		return err
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	ordered := append([]savedWindowsDACL(nil), saved...)
	sort.Slice(ordered, func(left, right int) bool {
		return len(ordered[left].path) > len(ordered[right].path)
	})
	for _, item := range ordered {
		if err := windows.SetNamedSecurityInfo(
			item.path,
			windows.SE_FILE_OBJECT,
			windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
			nil, nil, dacl, nil,
		); err != nil {
			return err
		}
	}
	return nil
}

func restoreWindowsDACLTree(saved []savedWindowsDACL) error {
	ordered := append([]savedWindowsDACL(nil), saved...)
	sort.Slice(ordered, func(left, right int) bool {
		return len(ordered[left].path) < len(ordered[right].path)
	})
	var failures []error
	for _, item := range ordered {
		dacl, _, err := item.descriptor.DACL()
		if err != nil {
			failures = append(failures, err)
			continue
		}
		information := windows.SECURITY_INFORMATION(
			windows.DACL_SECURITY_INFORMATION | windows.UNPROTECTED_DACL_SECURITY_INFORMATION,
		)
		if item.protected {
			information = windows.SECURITY_INFORMATION(
				windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION,
			)
		}
		if err := windows.SetNamedSecurityInfo(
			item.path, windows.SE_FILE_OBJECT, information,
			nil, nil, dacl, nil,
		); err != nil {
			failures = append(failures, err)
		}
	}
	for _, item := range ordered {
		current, err := windows.GetNamedSecurityInfo(
			item.path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION,
		)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		control, _, err := current.Control()
		if err != nil {
			failures = append(failures, err)
			continue
		}
		protected := control&windows.SE_DACL_PROTECTED != 0
		if current.String() != item.daclString || protected != item.protected {
			failures = append(failures, errors.New("backup: restored Windows DACL differs from original"))
		}
	}
	return errors.Join(failures...)
}
