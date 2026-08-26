//go:build windows

package browserqualification

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func protectTestArtifactBundle(t *testing.T, root string, approval artifactApproval) {
	t.Helper()
	for _, path := range []string{
		filepath.Join(root, approval.Browser.FileName),
		filepath.Join(root, approval.Driver.FileName),
		filepath.Join(root, approval.MindWeaver.FileName),
		root,
	} {
		if err := setTestArtifactDACL(path, false); err != nil {
			t.Fatalf("protect test artifact bundle: %v", err)
		}
	}
}

func expandTestArtifactACL(path string) error {
	return setTestArtifactDACL(path, true)
}

func verifyTestArtifactBundleProtection(t *testing.T, root string, approval artifactApproval) {
	t.Helper()
	for _, item := range []struct {
		path      string
		directory bool
	}{
		{root, true},
		{filepath.Join(root, approval.Browser.FileName), false},
		{filepath.Join(root, approval.Driver.FileName), false},
		{filepath.Join(root, approval.MindWeaver.FileName), false},
	} {
		file, err := openApprovedArtifactHandle(item.path, item.directory)
		if err != nil {
			t.Fatalf("open protected test artifact: %v", err)
		}
		fixed := fixedLocalArtifactHandle(windows.Handle(file.Fd()), item.path)
		owner := ownerOnlyArtifactACL(windows.Handle(file.Fd()))
		infoErr := verifyApprovedArtifactHandle(file, item.directory)
		closeErr := file.Close()
		if !fixed || !owner || infoErr != nil {
			t.Fatalf("protected test artifact classification: fixed=%t owner=%t verify=%v", fixed, owner, infoErr)
		}
		if closeErr != nil {
			t.Fatalf("close protected test artifact: %v", closeErr)
		}
	}
}

func setTestArtifactDACL(path string, includeWorld bool) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil {
		return fmt.Errorf("read test owner SID: %w", err)
	}
	sddl := "D:P(A;;GA;;;" + user.User.Sid.String() + ")"
	if includeWorld {
		sddl += "(A;;GR;;;WD)"
	}
	descriptor, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return err
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil)
}

func TestApprovedArtifactRejectsOfflineAttribute(t *testing.T) {
	approval := testArtifactApproval()
	root := writeArtifactBundle(t, approval)
	path := filepath.Join(root, approval.Driver.FileName)
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetFileAttributes(pointer, windows.FILE_ATTRIBUTE_OFFLINE); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = windows.SetFileAttributes(pointer, windows.FILE_ATTRIBUTE_NORMAL) })
	report := RunQualification(context.Background(), Approval{artifact: &approval}, RunOptions{
		SourceRevision: testRevision, BundleRoot: root, Now: fixedClock(),
	})
	assertBlocked(t, report, BlockerArtifactBundleInvalid)
}
